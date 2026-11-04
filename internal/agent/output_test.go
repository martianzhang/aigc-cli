package agent

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapToolOutput(t *testing.T) {
	if got := CapToolOutput("small"); got != "small" {
		t.Errorf("CapToolOutput(small) = %q, want unchanged", got)
	}

	big := strings.Repeat("x", MaxToolOutputBytes+100)
	got := CapToolOutput(big)
	if !strings.Contains(got, "[output truncated:") {
		t.Errorf("CapToolOutput(big) missing truncation marker")
	}
	if len(got) > MaxToolOutputBytes+256 {
		t.Errorf("CapToolOutput(big) len = %d, want <= %d", len(got), MaxToolOutputBytes+256)
	}
}

func TestCapToolOutputRuneBoundary(t *testing.T) {
	big := strings.Repeat("你", MaxToolOutputBytes/3+10)
	got := CapToolOutput(big)
	if !strings.Contains(got, "[output truncated:") {
		t.Fatalf("expected truncation")
	}
	// The cut must not leave a partial UTF-8 sequence before the marker.
	body := got[:strings.Index(got, "\n\n[output truncated:")]
	if !utf8.ValidString(body) {
		t.Errorf("CapToolOutput split a multi-byte rune")
	}
}

func TestCappedOutput(t *testing.T) {
	w := newCappedOutput(10)
	if n, err := w.Write([]byte("0123456789abcdef")); n != 16 || err != nil {
		t.Fatalf("Write = (%d, %v), want (16, nil)", n, err)
	}
	got := w.String()
	if !strings.Contains(got, "0123456789") || !strings.Contains(got, "[output truncated: showing 10 of 16 bytes") {
		t.Errorf("capped output = %q", got)
	}
}

func TestCappedOutputNoTruncation(t *testing.T) {
	w := newCappedOutput(10)
	if _, err := w.Write([]byte("short")); err != nil {
		t.Fatal(err)
	}
	if got := w.String(); got != "short" {
		t.Errorf("capped output = %q, want short", got)
	}
}

func TestShellCommandTruncatesLargeOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX pipeline used to generate a large stream")
	}
	got := ShellCommand("head -c 2000000 /dev/zero | tr '\\0' x")
	if !strings.Contains(got, "[output truncated:") {
		t.Fatalf("expected shell output truncation marker, got %d bytes", len(got))
	}
	if len(got) > MaxToolOutputBytes+512 {
		t.Errorf("truncated shell output = %d bytes, want <= %d", len(got), MaxToolOutputBytes+512)
	}
}
