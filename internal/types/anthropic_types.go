package types

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
type AnthropicMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"` // text content (simplified for text-only)
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
