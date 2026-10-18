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

func TestChatBody_messageWithImageBecomesParts(t *testing.T) {
	got, err := ChatBody(&types.ChatRequest{
		Model: "m",
		Messages: []types.ChatMessage{{
			Role:    "user",
			Content: "explain",
			Images:  []types.ChatImage{{Name: "a.png", MIMEType: "image/png", Data: []byte("abc")}},
		}},
	}, false, "")
	if err != nil {
		t.Fatalf("ChatBody: %v", err)
	}
	var body struct {
		Messages []struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, got)
	}
	if len(body.Messages) != 1 || len(body.Messages[0].Content) != 2 {
		t.Fatalf("body = %s, want one message with two parts", got)
	}
	if u := body.Messages[0].Content[1].ImageURL.URL; u != "data:image/png;base64,YWJj" {
		t.Errorf("image url = %q", u)
	}
}

func TestAnthropicChatBody_messageWithImage(t *testing.T) {
	got, err := AnthropicChatBody(&types.ChatRequest{
		Model: "claude",
		Messages: []types.ChatMessage{{
			Role:    "user",
			Content: "look",
			Images:  []types.ChatImage{{MIMEType: "image/png", Data: []byte("abc")}},
		}},
	})
	if err != nil {
		t.Fatalf("AnthropicChatBody: %v", err)
	}
	var body struct {
		Messages []struct {
			Content []struct {
				Type   string `json:"type"`
				Source struct {
					Type      string `json:"type"`
					MediaType string `json:"media_type"`
					Data      string `json:"data"`
				} `json:"source"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, got)
	}
	if len(body.Messages) != 1 || len(body.Messages[0].Content) != 2 {
		t.Fatalf("body = %s, want text + image blocks", got)
	}
	src := body.Messages[0].Content[1].Source
	if src.Type != "base64" || src.MediaType != "image/png" || src.Data != "YWJj" {
		t.Errorf("image source = %+v", src)
	}
}
