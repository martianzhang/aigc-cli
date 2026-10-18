package types

import (
	"encoding/base64"
	"encoding/json"
)

// AnthropicMessageRequest is the request body for POST /v1/messages (Anthropic Messages API).
type AnthropicMessageRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Messages    []AnthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	StopSeq     []string           `json:"stop_sequences,omitempty"`
}

// AnthropicMessage represents a single message in the conversation.
// MarshalJSON emits text-only content as a plain string (unchanged from before)
// and multimodal content as an array of text/image blocks.
type AnthropicMessage struct {
	Role    string
	Content string
	Images  []ChatImage
}

// MarshalJSON renders the Anthropic wire shape: `content` is a string for
// text-only messages, or an array of blocks ({"type":"text"} / {"type":"image"}
// with a base64 source) when images are attached.
func (m AnthropicMessage) MarshalJSON() ([]byte, error) {
	type wire struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}

	var content json.RawMessage
	if len(m.Images) == 0 {
		raw, err := json.Marshal(m.Content)
		if err != nil {
			return nil, err
		}
		content = raw
	} else {
		blocks := make([]map[string]any, 0, len(m.Images)+1)
		if m.Content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
		}
		for _, img := range m.Images {
			blocks = append(blocks, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": img.MIMEType,
					"data":       base64.StdEncoding.EncodeToString(img.Data),
				},
			})
		}
		raw, err := json.Marshal(blocks)
		if err != nil {
			return nil, err
		}
		content = raw
	}
	return json.Marshal(wire{Role: m.Role, Content: content})
}

// AnthropicMessageResponse is the non-streaming response from POST /v1/messages.
type AnthropicMessageResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"` // "message"
	Role       string                  `json:"role"` // "assistant"
	Content    []AnthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      *AnthropicUsage         `json:"usage,omitempty"`
}

// AnthropicContentBlock is a block within the response content array.
type AnthropicContentBlock struct {
	Type string `json:"type"` // "text" | "tool_use"
	Text string `json:"text,omitempty"`
}

// AnthropicUsage holds token usage information (Anthropic format).
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
