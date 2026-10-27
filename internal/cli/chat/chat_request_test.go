package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestBuildChatCurlVerbatim(t *testing.T) {
	options.Shared.APIKey = "test-key"
	options.Shared.APIBase = "https://api.apimart.ai/v1"
	options.Shared.APIKeySet = true
	options.Shared.APIBaseSet = true
	options.Shared.ProviderSet = false
	t.Cleanup(func() {
		options.Shared.APIKeySet = false
		options.Shared.APIBaseSet = false
	})

	const raw = `{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`
	req := &types.ChatRequest{RawJSON: []byte(raw)}
	p := options.Shared.ResolveProvider(options.ProviderNameChat)

	curl := buildChatCurl(req, p)
	if !strings.Contains(curl, "https://api.apimart.ai/v1/chat/completions") {
		t.Errorf("curl should target the chat endpoint, got:\n%s", curl)
	}
	if !strings.Contains(curl, raw) {
		t.Errorf("curl should forward the --json body verbatim, got:\n%s", curl)
	}
	if !strings.Contains(curl, "...-key") {
		t.Errorf("curl should mask the API key, got:\n%s", curl)
	}
}

func TestBuildChatRequestReadsPipedStdin(t *testing.T) {
	prevReader, prevTerminal := chatStdin, chatStdinIsTerminal
	chatStdin = strings.NewReader("  hello from stdin\n")
	chatStdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { chatStdin, chatStdinIsTerminal = prevReader, prevTerminal })

	tc := newChatTestCmd(t)
	setTestFlag(t, tc.cmd.Flags(), "system", "sys")

	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest: %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("Messages = %+v, want system+user", req.Messages)
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "sys" {
		t.Errorf("Messages[0] = %+v, want system message", req.Messages[0])
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != "hello from stdin" {
		t.Errorf("Messages[1] = %+v, want trimmed stdin message", req.Messages[1])
	}
}

func TestBuildChatRequestAppendsStdinToMessageFlag(t *testing.T) {
	prevReader, prevTerminal := chatStdin, chatStdinIsTerminal
	chatStdin = strings.NewReader("from stdin")
	chatStdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { chatStdin, chatStdinIsTerminal = prevReader, prevTerminal })

	tc := newChatTestCmd(t)
	setTestFlag(t, tc.cmd.Flags(), "message", "from flag")
	// StringArray flags accumulate, so add the second --message explicitly.
	chatMessages = []string{"from flag", "second flag"}
	t.Cleanup(func() { chatMessages = nil })

	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest: %v", err)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("Messages = %+v, want two --message values plus stdin", req.Messages)
	}
	want := []string{"from flag", "second flag", "from stdin"}
	for i, content := range want {
		if req.Messages[i].Role != "user" || req.Messages[i].Content != content {
			t.Errorf("Messages[%d] = %+v, want user %q", i, req.Messages[i], content)
		}
	}
}

func TestBuildChatRequestEmptyStdinAddsNoMessage(t *testing.T) {
	prevReader, prevTerminal := chatStdin, chatStdinIsTerminal
	chatStdin = strings.NewReader("   \n")
	chatStdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { chatStdin, chatStdinIsTerminal = prevReader, prevTerminal })

	tc := newChatTestCmd(t)
	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest: %v", err)
	}
	if len(req.Messages) != 0 {
		t.Errorf("Messages = %+v, want empty", req.Messages)
	}
}

func TestBuildChatRequestTerminalDoesNotReadStdin(t *testing.T) {
	prevReader, prevTerminal := chatStdin, chatStdinIsTerminal
	// A reader that would fail the test if consumed.
	chatStdin = panicReader{t: t}
	chatStdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { chatStdin, chatStdinIsTerminal = prevReader, prevTerminal })

	tc := newChatTestCmd(t)
	if _, err := buildChatRequest(tc.cmd); err != nil {
		t.Fatalf("buildChatRequest: %v", err)
	}
}

func TestBuildChatRequestJSONRejectsImageMention(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(imgPath, pngMagic, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tc := newChatTestCmd(t)
	setTestFlag(t, tc.cmd.Flags(), "json", `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	setTestFlag(t, tc.cmd.Flags(), "message", "what is @"+imgPath)

	_, err := buildChatRequest(tc.cmd)
	if err == nil {
		t.Fatal("buildChatRequest() error = nil, want --json + image mention rejected")
	}
	// The error must tell the user what to do instead of leaking a Go type error.
	for _, want := range []string{"--json", "pic.png", "--message"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestBuildChatRequestJSONTextMentionStillInlines(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(txtPath, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tc := newChatTestCmd(t)
	setTestFlag(t, tc.cmd.Flags(), "json", `{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	setTestFlag(t, tc.cmd.Flags(), "message", "explain @"+txtPath)

	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest: %v", err)
	}
	if !strings.Contains(string(req.RawJSON), "hello world") {
		t.Errorf("RawJSON = %s, want the inlined text file", req.RawJSON)
	}
}

func TestBuildChatRequestJSONRejectsContentParts(t *testing.T) {
	const body = `{"model":"m","messages":[{"role":"user","content":[` +
		`{"type":"text","text":"describe"},` +
		`{"type":"image_url","image_url":{"url":"data:image/png;base64,AAA"}}]}]}`

	tc := newChatTestCmd(t)
	setTestFlag(t, tc.cmd.Flags(), "json", body)

	_, err := buildChatRequest(tc.cmd)
	if err == nil {
		t.Fatal("buildChatRequest() error = nil, want content parts rejected")
	}
	for _, want := range []string{"messages[0].content", "--inject-json"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

type panicReader struct{ t *testing.T }

func (r panicReader) Read([]byte) (int, error) {
	r.t.Fatal("stdin should not be read in terminal mode")
	return 0, nil
}
