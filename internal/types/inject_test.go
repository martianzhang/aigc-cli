package types

import (
	"encoding/json"
	"testing"
)

// TestChatRequestInjectJSONArraysAppend verifies --inject-json merges on top of
// a verbatim --json body and that arrays append rather than replace.
func TestChatRequestInjectJSONArraysAppend(t *testing.T) {
	req := ChatRequest{
		RawJSON:    json.RawMessage(`{"model":"m","tools":[{"type":"function","function":{"name":"a"}}]}`),
		InjectJSON: json.RawMessage(`{"tools":[{"type":"web_search_preview"}],"enable_search":true}`),
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got struct {
		Model        string `json:"model"`
		EnableSearch bool   `json:"enable_search"`
		Tools        []struct {
			Type string `json:"type"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", out, err)
	}
	if got.Model != "m" {
		t.Errorf("model = %q, want m", got.Model)
	}
	if !got.EnableSearch {
		t.Errorf("enable_search = false, want true")
	}
	if len(got.Tools) != 2 {
		t.Fatalf("tools length = %d, want 2 (append): %s", len(got.Tools), out)
	}
	if got.Tools[0].Type != "function" || got.Tools[1].Type != "web_search_preview" {
		t.Errorf("tools = %+v, want [function, web_search_preview]", got.Tools)
	}
}

// TestGenerateRequestInjectJSONLayersOnTypedFields verifies inject is applied
// even when the body comes from typed fields (no --json).
func TestGenerateRequestInjectJSONLayersOnTypedFields(t *testing.T) {
	req := GenerateRequest{
		Model:      "m",
		Prompt:     "p",
		InjectJSON: json.RawMessage(`{"enable_search":true}`),
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", out, err)
	}
	if got["model"] != "m" || got["prompt"] != "p" {
		t.Errorf("typed fields lost: %s", out)
	}
	if got["enable_search"] != true {
		t.Errorf("enable_search = %v, want true: %s", got["enable_search"], out)
	}
}

// TestInjectJSONEmptyLeavesBodyUntouched verifies the no-inject path is
// byte-identical to the existing verbatim --json passthrough.
func TestInjectJSONEmptyLeavesBodyUntouched(t *testing.T) {
	const raw = `{"model":"m","custom_x":9}`
	out, err := json.Marshal(GenerateRequest{RawJSON: json.RawMessage(raw)})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(out) != raw {
		t.Errorf("body = %s, want %s", out, raw)
	}
}

// TestInjectJSONInvalidFailsMarshal verifies a non-object inject surfaces as a
// marshal error instead of silently producing a wrong body.
func TestInjectJSONInvalidFailsMarshal(t *testing.T) {
	_, err := json.Marshal(ChatRequest{
		RawJSON:    json.RawMessage(`{"model":"m"}`),
		InjectJSON: json.RawMessage(`[1,2]`),
	})
	if err == nil {
		t.Fatal("Marshal() = nil error, want error for a non-object inject")
	}
}
