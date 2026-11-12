package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/cliutil"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// TestNonInteractiveToolConfirmationFailsClosed covers the non-interactive
// policy: confirm-gated tools are blocked unless --yes is set, and an explicit
// confirm_tools: [] disables the gate entirely.
func TestNonInteractiveToolConfirmationFailsClosed(t *testing.T) {
	tests := []struct {
		name        string
		yes         bool
		cfg         *types.Config
		wantBlocked bool
	}{
		{
			name:        "default blocks without --yes",
			wantBlocked: true,
		},
		{
			name: "yes allows the tool",
			yes:  true,
		},
		{
			name: "empty confirm_tools disables the gate",
			cfg: &types.Config{Defaults: &types.ConfigDefaults{
				Chat: &types.ChatDefaults{ConfirmTools: []string{}},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if atomic.AddInt32(&calls, 1) == 1 {
					args, err := json.Marshal(map[string]string{"command": "echo hello"})
					if err != nil {
						t.Fatalf("marshal tool arguments: %v", err)
					}
					quotedArgs, err := json.Marshal(string(args))
					if err != nil {
						t.Fatalf("quote tool arguments: %v", err)
					}
					fmt.Fprintf(w, `{"choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":%s}}]},"finish_reason":"tool_calls"}]}`, quotedArgs)
					return
				}
				fmt.Fprint(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()

			prevModel, prevNoStream, prevYes, prevCfg := options.Shared.Model, chatNoStream, chatYes, options.Shared.Cfg
			options.Shared.Model, chatNoStream, chatYes, options.Shared.Cfg = "test-model", true, tt.yes, tt.cfg
			var errBuf bytes.Buffer
			restoreOut := cliutil.SetStdout(io.Discard)
			restoreErr := cliutil.SetStderr(&errBuf)
			t.Cleanup(func() {
				options.Shared.Model, chatNoStream, chatYes, options.Shared.Cfg = prevModel, prevNoStream, prevYes, prevCfg
				restoreOut()
				restoreErr()
			})

			c := client.NewWithProvider("sk-test", server.URL, "", types.ProviderOpenAI)
			history := []types.ChatMessage{{Role: "user", Content: "hi"}}
			if err := runAgentLoop(context.Background(), c, nil, &history, 3, newAgentLoopTestCmd(), nil); err != nil {
				t.Fatalf("runAgentLoop() error = %v", err)
			}

			var toolResult string
			for _, msg := range history {
				if msg.Role == "tool" {
					toolResult = msg.Content
				}
			}
			if tt.wantBlocked {
				if !strings.Contains(toolResult, "blocked") || !strings.Contains(toolResult, "--yes") {
					t.Errorf("tool result = %q, want a blocked result mentioning --yes", toolResult)
				}
				if !strings.Contains(errBuf.String(), "[tool] blocked") {
					t.Errorf("stderr = %q, want the blocked notice", errBuf.String())
				}
				return
			}
			if strings.Contains(toolResult, "blocked") {
				t.Errorf("tool result = %q, want the command output instead of a block", toolResult)
			}
			if !strings.Contains(toolResult, "hello") {
				t.Errorf("tool result = %q, want the command output", toolResult)
			}
		})
	}
}
