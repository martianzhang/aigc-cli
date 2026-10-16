package client

import (
	"encoding/json"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func decodeObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal body %s: %v", body, err)
	}
	return got
}

// TestAnthropicChatBodyInjectJSON verifies provider-native chat bodies that do
// not route through MarshalJSON still honor --inject-json.
func TestAnthropicChatBodyInjectJSON(t *testing.T) {
	req := &types.ChatRequest{
		Model:      "claude",
		Messages:   []types.ChatMessage{{Role: "user", Content: "hi"}},
		InjectJSON: json.RawMessage(`{"tools":[{"type":"web_search_20250305","name":"web_search"}]}`),
	}
	body, err := AnthropicChatBody(req)
	if err != nil {
		t.Fatalf("AnthropicChatBody() error = %v", err)
	}
	got := decodeObject(t, body)
	if got["model"] != "claude" {
		t.Errorf("model = %v, want claude", got["model"])
	}
	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want one injected server tool", got["tools"])
	}
}

func TestResponsesBodyInjectJSON(t *testing.T) {
	req := &types.ChatRequest{
		Model:      "gpt-5",
		Messages:   []types.ChatMessage{{Role: "user", Content: "hi"}},
		InjectJSON: json.RawMessage(`{"tools":[{"type":"web_search_preview"}]}`),
	}
	body, err := ResponsesBody(req)
	if err != nil {
		t.Fatalf("ResponsesBody() error = %v", err)
	}
	got := decodeObject(t, body)
	if got["model"] != "gpt-5" {
		t.Errorf("model = %v, want gpt-5", got["model"])
	}
	tools, ok := got["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want one injected web_search_preview", got["tools"])
	}
}

// TestOpenRouterImageBodyInjectJSON verifies the BodyOrRaw path layers inject
// on top of a provider-native map.
func TestOpenRouterImageBodyInjectJSON(t *testing.T) {
	req := &types.GenerateRequest{
		Model:      "m",
		Prompt:     "p",
		InjectJSON: json.RawMessage(`{"plugins":[{"id":"web"}]}`),
	}
	body, err := OpenRouterImageBody(req)
	if err != nil {
		t.Fatalf("OpenRouterImageBody() error = %v", err)
	}
	got := decodeObject(t, marshalBytes(t, body))
	if got["model"] != "m" || got["prompt"] != "p" {
		t.Errorf("provider fields lost: %#v", got)
	}
	if _, ok := got["plugins"]; !ok {
		t.Errorf("injected plugins missing: %#v", got)
	}
}

func marshalBytes(t *testing.T, body any) []byte {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return data
}
