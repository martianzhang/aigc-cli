package client

import (
	"reflect"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

// ---------------------------------------------------------------------------
// buildAnthropicRequest tests
// ---------------------------------------------------------------------------

func TestBuildAnthropicRequest_systemExtracted(t *testing.T) {
	c := New("sk-test", "https://api.anthropic.com", "")
	req := &types.ChatRequest{
		Model: "claude-3-opus",
		Messages: []types.ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello"},
		},
	}

	got := c.buildAnthropicRequest(req)

	if got.System != "You are a helpful assistant." {
		t.Errorf("System = %q, want %q", got.System, "You are a helpful assistant.")
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Errorf("Messages = %+v, want 1 user message", got.Messages)
	}
}

func TestBuildAnthropicRequest_normalMessages(t *testing.T) {
	c := New("sk-test", "https://api.anthropic.com", "")
	req := &types.ChatRequest{
		Model: "claude-3-haiku",
		Messages: []types.ChatMessage{
			{Role: "user", Content: "Hi"},
			{Role: "assistant", Content: "Hello!"},
			{Role: "user", Content: "Bye"},
		},
	}

	got := c.buildAnthropicRequest(req)

	if len(got.Messages) != 3 {
		t.Fatalf("Messages length = %d, want 3", len(got.Messages))
	}
	wantRoles := []string{"user", "assistant", "user"}
	for i, want := range wantRoles {
		if got.Messages[i].Role != want {
			t.Errorf("Messages[%d].Role = %q, want %q", i, got.Messages[i].Role, want)
		}
	}
	if got.System != "" {
		t.Errorf("System should be empty when no system message, got %q", got.System)
	}
}

func TestBuildAnthropicRequest_maxTokens(t *testing.T) {
	c := New("sk-test", "https://api.anthropic.com", "")

	tests := []struct {
		name      string
		maxTokens *int
		want      int
	}{
		{"nil defaults to 1024", nil, 1024},
		{"zero defaults to 1024", intPtr(0), 1024},
		{"positive value preserved", intPtr(2048), 2048},
		{"small positive value", intPtr(100), 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &types.ChatRequest{Model: "claude-3", MaxTokens: tt.maxTokens}
			got := c.buildAnthropicRequest(req)
			if got.MaxTokens != tt.want {
				t.Errorf("MaxTokens = %d, want %d", got.MaxTokens, tt.want)
			}
		})
	}
}

func TestBuildAnthropicRequest_temperature(t *testing.T) {
	c := New("sk-test", "https://api.anthropic.com", "")

	tests := []struct {
		name        string
		temperature *float64
		wantSet     bool
		wantValue   float64
	}{
		{"nil means not set", nil, false, 0},
		{"0.5 preserved", floatPtr(0.5), true, 0.5},
		{"0.0 preserved", floatPtr(0.0), true, 0.0},
		{"1.0 preserved", floatPtr(1.0), true, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &types.ChatRequest{Model: "claude-3", Temperature: tt.temperature}
			got := c.buildAnthropicRequest(req)
			if tt.wantSet {
				if got.Temperature == nil {
					t.Fatal("Temperature is nil, want a value")
				}
				if *got.Temperature != tt.wantValue {
					t.Errorf("Temperature = %v, want %v", *got.Temperature, tt.wantValue)
				}
			} else {
				if got.Temperature != nil {
					t.Errorf("Temperature = %v, want nil", *got.Temperature)
				}
			}
		})
	}
}

func TestBuildAnthropicRequest_emptyMessages(t *testing.T) {
	c := New("sk-test", "https://api.anthropic.com", "")
	req := &types.ChatRequest{Model: "claude-3", Messages: nil}

	got := c.buildAnthropicRequest(req)

	if got.Messages == nil {
		t.Error("Messages should be empty slice, not nil")
	}
	if len(got.Messages) != 0 {
		t.Errorf("Messages length = %d, want 0", len(got.Messages))
	}
}

func TestBuildAnthropicRequest_streamFlag(t *testing.T) {
	c := New("sk-test", "https://api.anthropic.com", "")

	tests := []struct {
		name   string
		stream bool
	}{
		{"stream true", true},
		{"stream false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &types.ChatRequest{Model: "claude-3", Stream: tt.stream}
			got := c.buildAnthropicRequest(req)
			if got.Stream != tt.stream {
				t.Errorf("Stream = %v, want %v", got.Stream, tt.stream)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// anthropicToChatResponse tests
// ---------------------------------------------------------------------------

func TestAnthropicToChatResponse_textConcatenation(t *testing.T) {
	ar := &types.AnthropicMessageResponse{
		ID:    "msg_01",
		Model: "claude-3-opus",
		Content: []types.AnthropicContentBlock{
			{Type: "text", Text: "Hello "},
			{Type: "text", Text: "world"},
			{Type: "text", Text: "!"},
		},
	}

	got := anthropicToChatResponse(ar)

	wantContent := "Hello world!"
	if got.Choices[0].Message.Content != wantContent {
		t.Errorf("Content = %q, want %q", got.Choices[0].Message.Content, wantContent)
	}
}

func TestAnthropicToChatResponse_ignoresNonText(t *testing.T) {
	ar := &types.AnthropicMessageResponse{
		ID:    "msg_02",
		Model: "claude-3-haiku",
		Content: []types.AnthropicContentBlock{
			{Type: "text", Text: "The answer is "},
			{Type: "tool_use", Text: "should_be_ignored"},
			{Type: "text", Text: "42."},
		},
	}

	got := anthropicToChatResponse(ar)

	wantContent := "The answer is 42."
	if got.Choices[0].Message.Content != wantContent {
		t.Errorf("Content = %q, want %q", got.Choices[0].Message.Content, wantContent)
	}
}

func TestAnthropicToChatResponse_usageMapping(t *testing.T) {
	tests := []struct {
		name      string
		usage     *types.AnthropicUsage
		wantUsage *types.ChatUsage
	}{
		{
			name: "usage present",
			usage: &types.AnthropicUsage{
				InputTokens:  10,
				OutputTokens: 20,
			},
			wantUsage: &types.ChatUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		},
		{
			name:      "nil usage",
			usage:     nil,
			wantUsage: nil,
		},
		{
			name: "zero tokens",
			usage: &types.AnthropicUsage{
				InputTokens:  0,
				OutputTokens: 0,
			},
			wantUsage: &types.ChatUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ar := &types.AnthropicMessageResponse{
				ID:      "msg_03",
				Model:   "claude-3",
				Content: []types.AnthropicContentBlock{{Type: "text", Text: "ok"}},
				Usage:   tt.usage,
			}
			got := anthropicToChatResponse(ar)
			if !reflect.DeepEqual(got.Usage, tt.wantUsage) {
				t.Errorf("Usage = %+v, want %+v", got.Usage, tt.wantUsage)
			}
		})
	}
}

func TestAnthropicToChatResponse_propagatesIDAndModel(t *testing.T) {
	tests := []struct {
		id    string
		model string
	}{
		{"msg_abc123", "claude-3-opus-20240229"},
		{"msg_xyz", "claude-3-5-sonnet-20241022"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			ar := &types.AnthropicMessageResponse{
				ID:      tt.id,
				Model:   tt.model,
				Content: []types.AnthropicContentBlock{{Type: "text", Text: "hi"}},
			}
			got := anthropicToChatResponse(ar)
			if got.ID != tt.id {
				t.Errorf("ID = %q, want %q", got.ID, tt.id)
			}
			if got.Model != tt.model {
				t.Errorf("Model = %q, want %q", got.Model, tt.model)
			}
			if got.Object != "chat.completion" {
				t.Errorf("Object = %q, want %q", got.Object, "chat.completion")
			}
		})
	}
}
