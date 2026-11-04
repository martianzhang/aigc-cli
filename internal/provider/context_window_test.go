package provider

import "testing"

func TestBuiltinContextWindow(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		{"gpt-4o", 128000},
		{"gpt-4o-2024-08-06", 128000},
		{"openai/gpt-4.1", 1047576},
		{"anthropic/claude-sonnet-4", 200000},
		{"gemini-2.5-pro", 1048576},
		{"deepseek-chat", 128000},
		{"qwen-max", 131072},
		{"unknown-model", 0},
		{"", 0},
	}
	for _, c := range cases {
		if got := BuiltinContextWindow(c.model); got != c.want {
			t.Errorf("BuiltinContextWindow(%q) = %d, want %d", c.model, got, c.want)
		}
	}
}
