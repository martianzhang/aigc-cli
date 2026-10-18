package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatMessageMarshalJSON_textOnlyUnchanged(t *testing.T) {
	got, err := json.Marshal(ChatMessage{Role: "user", Content: "hi"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != `{"role":"user","content":"hi"}` {
		t.Errorf("text-only body = %s, want the plain string form", got)
	}
}

func TestChatMessageMarshalJSON_omitsEmptyContent(t *testing.T) {
	got, err := json.Marshal(ChatMessage{Role: "assistant"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(got), "content") {
		t.Errorf("body = %s, want no content key", got)
	}
}

func TestChatMessageMarshalJSON_toolFieldsPreserved(t *testing.T) {
	got, err := json.Marshal(ChatMessage{
		Role:       "tool",
		Content:    "result",
		ToolCallID: "call_1",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"role":"tool"`, `"content":"result"`, `"tool_call_id":"call_1"`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("body missing %s: %s", want, got)
		}
	}
}

func TestChatMessageMarshalJSON_withImage(t *testing.T) {
	got, err := json.Marshal(ChatMessage{
		Role:    "user",
		Content: "what is this",
		Images:  []ChatImage{{Name: "a.png", MIMEType: "image/png", Data: []byte("abc")}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed struct {
		Role    string `json:"role"`
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL struct {
				URL string `json:"url"`
			} `json:"image_url"`
		} `json:"content"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, got)
	}
	if len(parsed.Content) != 2 {
		t.Fatalf("content parts = %d, want 2\nbody: %s", len(parsed.Content), got)
	}
	if parsed.Content[0].Type != "text" || parsed.Content[0].Text != "what is this" {
		t.Errorf("part[0] = %+v, want a text part", parsed.Content[0])
	}
	if parsed.Content[1].Type != "image_url" {
		t.Errorf("part[1].type = %q, want image_url", parsed.Content[1].Type)
	}
	if want := "data:image/png;base64,YWJj"; parsed.Content[1].ImageURL.URL != want {
		t.Errorf("image url = %q, want %q", parsed.Content[1].ImageURL.URL, want)
	}
}

func TestAnthropicMessageMarshalJSON(t *testing.T) {
	// Text-only keeps the plain-string form used before images existed.
	got, err := json.Marshal(AnthropicMessage{Role: "user", Content: "hi"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != `{"role":"user","content":"hi"}` {
		t.Errorf("text-only anthropic body = %s", got)
	}

	got, err = json.Marshal(AnthropicMessage{
		Role:    "user",
		Content: "look",
		Images:  []ChatImage{{MIMEType: "image/jpeg", Data: []byte("abc")}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed struct {
		Content []struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			Source struct {
				Type      string `json:"type"`
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			} `json:"source"`
		} `json:"content"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, got)
	}
	if len(parsed.Content) != 2 {
		t.Fatalf("blocks = %d, want 2\nbody: %s", len(parsed.Content), got)
	}
	if parsed.Content[0].Type != "text" || parsed.Content[0].Text != "look" {
		t.Errorf("block[0] = %+v, want text", parsed.Content[0])
	}
	src := parsed.Content[1].Source
	if parsed.Content[1].Type != "image" || src.Type != "base64" || src.MediaType != "image/jpeg" || src.Data != "YWJj" {
		t.Errorf("block[1] = %+v, want a base64 image block", parsed.Content[1])
	}
}
