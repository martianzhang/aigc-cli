package client

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestProgressBar_zero(t *testing.T) {
	got := progressBar(0, 20)
	if !strings.Contains(got, "░") {
		t.Errorf("progressBar(0, 20) should be all dots, got %q", got)
	}
}

func TestProgressBar_full(t *testing.T) {
	got := progressBar(100, 20)
	if !strings.Contains(got, "█") {
		t.Errorf("progressBar(100, 20) should be all blocks, got %q", got)
	}
	if strings.Contains(got, "░") {
		t.Errorf("progressBar(100, 20) should have no dots, got %q", got)
	}
}

func TestProgressBar_half(t *testing.T) {
	got := progressBar(50, 20)
	blockCount := strings.Count(got, "█")
	dotCount := strings.Count(got, "░")
	if blockCount != 10 {
		t.Errorf("progressBar(50, 20) should have 10 blocks, got %d", blockCount)
	}
	if dotCount != 10 {
		t.Errorf("progressBar(50, 20) should have 10 dots, got %d", dotCount)
	}
}

func TestProgressBar_customWidth(t *testing.T) {
	got := progressBar(25, 40)
	blockCount := strings.Count(got, "█")
	if blockCount != 10 {
		t.Errorf("progressBar(25, 40) should have 10 blocks, got %d", blockCount)
	}
}

func TestProgressBar_edgeCases(t *testing.T) {
	tests := []struct {
		pct   int
		width int
	}{
		{0, 10},
		{1, 10},
		{99, 10},
		{100, 10},
		{50, 1},
		{50, 100},
	}
	for _, tt := range tests {
		got := progressBar(tt.pct, tt.width)
		if utf8.RuneCountInString(got) != tt.width {
			t.Errorf("progressBar(%d, %d) length = %d, want %d", tt.pct, tt.width, len(got), tt.width)
		}
	}
}
