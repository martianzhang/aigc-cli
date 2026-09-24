package chat

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// newAgentLoopTestCmd builds the flags runAgentLoop reads via SetFloatFlag/SetIntFlag.
func newAgentLoopTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "chat"}
	cmd.Flags().Float64("temperature", 0, "")
	cmd.Flags().Int("max-output", 0, "")
	return cmd
}

// TestRunAgentLoop_noStreamPrintsOnce guards that --no-stream does not stream
// and that the single answer is printed exactly once by the loop (no duplicate).
func TestRunAgentLoop_noStreamPrintsOnce(t *testing.T) {
	var (
		mu   sync.Mutex
		body string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		body = string(b)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	prevModel, prevNoStream := options.Shared.Model, chatNoStream
	options.Shared.Model, chatNoStream = "test-model", true
	var out bytes.Buffer
	restoreOut := options.SetStdout(&out)
	restoreErr := options.SetStderr(io.Discard)
	t.Cleanup(func() {
		options.Shared.Model, chatNoStream = prevModel, prevNoStream
		restoreOut()
		restoreErr()
	})

	c := client.NewWithProvider("sk-test", srv.URL, "", types.ProviderOpenAI)
	history := []types.ChatMessage{{Role: "user", Content: "hi"}}

	if _, err := runAgentLoop(context.Background(), c, &history, nil, 3, newAgentLoopTestCmd()); err != nil {
		t.Fatalf("runAgentLoop() error = %v", err)
	}

	mu.Lock()
	gotBody := body
	mu.Unlock()
	if strings.Contains(gotBody, `"stream":true`) {
		t.Errorf("request body must not stream under --no-stream, got: %s", gotBody)
	}
	if n := strings.Count(out.String(), "hello"); n != 1 {
		t.Errorf("answer printed %d times, want 1 (stdout=%q)", n, out.String())
	}
}

// TestRunAgentLoop_noLeadingBlankLine guards that a single-turn answer is not
// preceded by a blank line on stderr (every turn used to emit "\r\n").
func TestRunAgentLoop_noLeadingBlankLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	prevModel, prevNoStream, prevVerbose := options.Shared.Model, chatNoStream, options.Shared.Verbose
	options.Shared.Model, chatNoStream, options.Shared.Verbose = "test-model", true, false
	var out, errBuf bytes.Buffer
	restoreOut := options.SetStdout(&out)
	restoreErr := options.SetStderr(&errBuf)
	t.Cleanup(func() {
		options.Shared.Model, chatNoStream, options.Shared.Verbose = prevModel, prevNoStream, prevVerbose
		restoreOut()
		restoreErr()
	})

	c := client.NewWithProvider("sk-test", srv.URL, "", types.ProviderOpenAI)
	history := []types.ChatMessage{{Role: "user", Content: "hi"}}

	if _, err := runAgentLoop(context.Background(), c, &history, nil, 3, newAgentLoopTestCmd()); err != nil {
		t.Fatalf("runAgentLoop() error = %v", err)
	}

	if errBuf.Len() != 0 {
		t.Errorf("stderr = %q, want empty (no leading blank line)", errBuf.String())
	}
	if got := out.String(); got != "hello\n" {
		t.Errorf("stdout = %q, want %q", got, "hello\n")
	}
}
