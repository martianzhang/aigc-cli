package client

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestChatRequestForwardsRawJSON(t *testing.T) {
	const raw = `{"model":"m","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`
	got, err := json.Marshal(&types.ChatRequest{RawJSON: []byte(raw)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != raw {
		t.Errorf("raw --json body = %s, want %s", got, raw)
	}
}

func TestChatRequestTypedFieldsUnchanged(t *testing.T) {
	got, err := json.Marshal(&types.ChatRequest{Model: "m", Messages: []types.ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"model":"m"`, `"messages":[{"role":"user","content":"hi"}]`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("typed body missing %s: %s", want, got)
		}
	}
	if strings.Contains(string(got), "RawJSON") {
		t.Errorf("RawJSON leaked into typed marshal: %s", got)
	}
}

func TestAnthropicChatBodyForwardsRawJSON(t *testing.T) {
	const raw = `{"model":"claude","max_tokens":16,"messages":[],"thinking":{"type":"enabled"}}`
	got, err := AnthropicChatBody(&types.ChatRequest{RawJSON: []byte(raw)})
	if err != nil {
		t.Fatalf("AnthropicChatBody: %v", err)
	}
	if string(got) != raw {
		t.Errorf("raw anthropic body = %s, want %s", got, raw)
	}
}
