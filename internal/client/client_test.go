package client

import (
	"os"
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

func TestIsLocalFile_exists(t *testing.T) {
	tmp, _ := os.CreateTemp("", "testfile")
	tmp.Close()
	defer os.Remove(tmp.Name())

	if !isLocalFile(tmp.Name()) {
		t.Errorf("isLocalFile(%q) should be true", tmp.Name())
	}
}

func TestIsLocalFile_notExists(t *testing.T) {
	if isLocalFile("/tmp/nonexistent_file_xyz") {
		t.Error("isLocalFile() should be false for nonexistent file")
	}
}

func TestIsLocalFile_directory(t *testing.T) {
	dir, _ := os.MkdirTemp("", "testdir")
	defer os.Remove(dir)

	if isLocalFile(dir) {
		t.Error("isLocalFile() should be false for directory")
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
