package cmd

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"short", "hello", 2},
		{"cjk", "你好世界", 6}, // 12 bytes / 2
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := estimateTokens(c.in); got != c.want {
				t.Errorf("estimateTokens(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestEstimateHistoryTokens(t *testing.T) {
	hist := []types.ChatMessage{
		{Role: "system", Content: "sys"},            // 1
		{Role: "user", Content: "hi"},               // 1
		{Role: "assistant", Content: "hello there"}, // 5
	}
	// 1 + 1 + 5 = 7
	if got := estimateHistoryTokens(hist); got != 7 {
		t.Errorf("estimateHistoryTokens = %d, want 7", got)
	}
}

func TestAutoCompactIfNeededBelowThreshold(t *testing.T) {
	// Small history (well under threshold) must be returned unchanged.
	hist := []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	got := autoCompactIfNeeded(nil, "model", 0, 0, 1000, hist)
	if len(got) != len(hist) {
		t.Fatalf("history changed below threshold: got %d msgs, want %d", len(got), len(hist))
	}
}

func TestAutoCompactIfNeededDisabled(t *testing.T) {
	// contextSize <= 0 means auto-compact disabled.
	hist := []types.ChatMessage{{Role: "user", Content: "x"}}
	got := autoCompactIfNeeded(nil, "model", 0, 0, 0, hist)
	if len(got) != len(hist) {
		t.Fatalf("history changed with contextSize=0")
	}
}
