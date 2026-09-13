package agent

import (
	"io"
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
		{"cjk", "你好世界", 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EstimateTokens(c.in); got != c.want {
				t.Errorf("EstimateTokens(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestEstimateHistoryTokens(t *testing.T) {
	hist := []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello there"},
	}
	if got := EstimateHistoryTokens(hist); got != 7 {
		t.Errorf("EstimateHistoryTokens = %d, want 7", got)
	}
}

func TestAutoCompactIfNeededBelowThreshold(t *testing.T) {
	hist := []types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	got := AutoCompactIfNeeded(nil, "model", 0, 0, 1000, hist, io.Discard)
	if len(got) != len(hist) {
		t.Fatalf("history changed below threshold: got %d msgs, want %d", len(got), len(hist))
	}
}

func TestAutoCompactIfNeededDisabled(t *testing.T) {
	hist := []types.ChatMessage{{Role: "user", Content: "x"}}
	got := AutoCompactIfNeeded(nil, "model", 0, 0, 0, hist, io.Discard)
	if len(got) != len(hist) {
		t.Fatalf("history changed with contextSize=0")
	}
}
