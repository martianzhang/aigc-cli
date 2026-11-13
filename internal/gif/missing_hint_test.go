package gif

import (
	"strings"
	"testing"
)

func TestMissingHintIsActionableAndShort(t *testing.T) {
	err := MissingHint()
	if err == nil {
		t.Fatal("MissingHint() = nil")
	}
	msg := err.Error()

	lines := strings.Split(msg, "\n")
	if len(lines) > 3 {
		t.Fatalf("MissingHint() has %d lines, want <=3:\n%s", len(lines), msg)
	}
	if !strings.Contains(msg, "Hint: ") {
		t.Fatalf("MissingHint() missing Hint line:\n%s", msg)
	}
	for _, want := range []string{"ffmpeg not found", "brew install ffmpeg", "winget install"} {
		if !strings.Contains(msg, want) {
			t.Errorf("MissingHint() missing %q:\n%s", want, msg)
		}
	}
}
