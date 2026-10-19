package types

import (
	"encoding/json"
	"strings"
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
// TestChatMessageAnnotationsAreNotSerialized guards that response-only
// citations never leak into a request body.
func TestChatMessageAnnotationsAreNotSerialized(t *testing.T) {
	msg := ChatMessage{
		Role:    "assistant",
		Content: "hi",
		Annotations: []ChatAnnotation{
			{Type: "url_citation", URL: "https://example.com", Title: "Example"},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(data), "annotations") {
		t.Errorf("request message = %s, want no annotations field", data)
	}
}

func TestChatResponseParsesAnnotationsAndWebSearchUsage(t *testing.T) {
	raw := `{
		"choices": [{"message": {"role": "assistant", "content": "answer",
			"annotations": [{"type": "url_citation", "url": "https://example.com/a",
				"title": "A", "site_name": "Site", "publish_time": "2026-01-01",
				"summary": "s", "logo_url": "https://example.com/logo.png"}]}}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12,
			"web_search_usage": {"tool_usage": 1, "page_usage": 5}}
	}`
	var resp ChatResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	anns := resp.Choices[0].Message.Annotations
	if len(anns) != 1 || anns[0].URL != "https://example.com/a" || anns[0].SiteName != "Site" {
		t.Errorf("Annotations = %+v, want the parsed citation", anns)
	}
	if resp.Usage == nil || resp.Usage.WebSearchUsage == nil {
		t.Fatal("WebSearchUsage = nil, want parsed counters")
	}
	if got := resp.Usage.WebSearchUsage.ToolUsage; got != 1 {
		t.Errorf("ToolUsage = %d, want 1", got)
	}
	if got := resp.Usage.WebSearchUsage.PageUsage; got != 5 {
		t.Errorf("PageUsage = %d, want 5", got)
	}
}

func TestInjectJSONInvalidFailsMarshal(t *testing.T) {
	_, err := json.Marshal(ChatRequest{
		RawJSON:    json.RawMessage(`{"model":"m"}`),
		InjectJSON: json.RawMessage(`[1,2]`),
	})
	if err == nil {
		t.Fatal("Marshal() = nil error, want error for a non-object inject")
	}
}
