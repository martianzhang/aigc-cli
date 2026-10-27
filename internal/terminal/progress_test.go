package terminal

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewDisabledWithoutTTY(t *testing.T) {
	p := New(&bytes.Buffer{})
	if p.Enabled() {
		t.Fatal("a non-*os.File writer must not enable OSC output")
	}
	p.Stage("image")
	p.Percent(50)
	p.Busy()
	p.Finish("done")
	p.Clear()
}

func TestNewDisabledByEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "plain iTerm2", env: map[string]string{"TERM_PROGRAM": "iTerm.app"}, want: true},
		{name: "xterm", env: map[string]string{"TERM": "xterm-256color"}, want: true},
		{name: "tmux", env: map[string]string{"TERM": "screen-256color"}, want: true},
		{name: "mintty", env: map[string]string{"TERM_PROGRAM": "mintty"}, want: true},
		{name: "windows console (no TERM)", env: map[string]string{}, want: true},
		{name: "dumb terminal", env: map[string]string{"TERM": "dumb"}, want: false},
		{name: "linux vt", env: map[string]string{"TERM": "linux"}, want: false},
		{name: "unknown TERM", env: map[string]string{"TERM": "cons25"}, want: false},
		{name: "unknown TERM forced on", env: map[string]string{"TERM": "cons25", EnvTitle: "1"}, want: true},
		{name: "opt-out", env: map[string]string{"TERM_PROGRAM": "iTerm.app", EnvDisable: "1"}, want: false},
		{name: "opt-out beats force", env: map[string]string{"TERM": "xterm", EnvTitle: "1", EnvDisable: "1"}, want: false},
	}
	for _, tc := range tests {
		var buf bytes.Buffer
		p := newEnabled(&buf, func(k string) string { return tc.env[k] })
		if p.Enabled() != tc.want {
			t.Errorf("%s: Enabled() = %v, want %v", tc.name, p.Enabled(), tc.want)
		}
	}
}

func TestSupportsProgressBar(t *testing.T) {
	tests := []struct {
		env  map[string]string
		want bool
	}{
		{env: map[string]string{"TERM_PROGRAM": "iTerm.app"}, want: true},
		{env: map[string]string{"LC_TERMINAL": "iTerm2"}, want: true},
		{env: map[string]string{"TERM_PROGRAM": "WezTerm"}, want: true},
		{env: map[string]string{"TERM_PROGRAM": "ghostty"}, want: true},
		{env: map[string]string{"WT_SESSION": "1"}, want: true}, // Windows Terminal
		{env: map[string]string{"TERM_PROGRAM": "vscode"}, want: true},
		{env: map[string]string{"TERM_PROGRAM": "mintty", "TERM": "xterm"}, want: false},
		{env: map[string]string{"TERM": "xterm-kitty"}, want: true},
		{env: map[string]string{"TERM_PROGRAM": "iTerm.app", "TMUX": "/tmp/tmux"}, want: false},
		{env: map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, want: false},
		{env: map[string]string{"TERM_PROGRAM": "Apple_Terminal", EnvTitleBar: "1"}, want: true},
	}
	for _, tc := range tests {
		got := supportsProgressBar(func(k string) string { return tc.env[k] })
		if got != tc.want {
			t.Errorf("supportsProgressBar(%v) = %v, want %v", tc.env, got, tc.want)
		}
	}
}

func TestTitleAndProgressSequences(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, enabled: true, bar: true}

	p.Stage("video")
	if got, want := buf.String(), "\x1b]0;aigc-cli · video\x07"; got != want {
		t.Errorf("Stage = %q, want %q", got, want)
	}

	buf.Reset()
	p.Percent(42)
	want := "\x1b]9;4;1;42\x07" + "\x1b]0;aigc-cli · video 42%\x07"
	if got := buf.String(); got != want {
		t.Errorf("Percent = %q, want %q", got, want)
	}

	buf.Reset()
	p.Busy()
	want = "\x1b]9;4;3\x07" + "\x1b]0;aigc-cli · video …\x07"
	if got := buf.String(); got != want {
		t.Errorf("Busy = %q, want %q", got, want)
	}

	buf.Reset()
	p.Finish("video complete")
	want = "\x1b]9;4;0\x07" + "\x1b]0;aigc-cli · video complete\x07"
	if got := buf.String(); got != want {
		t.Errorf("Finish = %q, want %q", got, want)
	}
	if p.stop != nil {
		t.Error("Finish must stop the keep-alive goroutine")
	}

	buf.Reset()
	p.Finish("")
	// The bar is already gone, so only the title changes.
	if got, want := buf.String(), "\x1b]0;aigc-cli\x07"; got != want {
		t.Errorf("Finish(\"\") = %q, want %q", got, want)
	}
}

func TestRepeatedUpdatesAreDeduped(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, enabled: true, bar: true}
	p.Stage("chat")
	p.Percent(40)
	buf.Reset()

	p.Stage("chat")
	p.Percent(40)
	if buf.Len() != 0 {
		t.Errorf("re-asserting the same state wrote %q, want nothing", buf.String())
	}

	p.Finish("chat")
	buf.Reset()
	p.Finish("chat")
	if buf.Len() != 0 {
		t.Errorf("second Finish wrote %q, want nothing", buf.String())
	}
}

func TestTitleOnlyTerminalOmitsProgressBar(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, enabled: true}
	p.Percent(10)
	if got := buf.String(); got != "\x1b]0;aigc-cli 10%\x07" {
		t.Errorf("got %q, want a title-only sequence", got)
	}
}

func TestPercentClamped(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, enabled: true}
	p.Percent(-5)
	if !strings.Contains(buf.String(), "aigc-cli\x07") {
		t.Errorf("Percent(-5) = %q, want no negative percentage", buf.String())
	}
	buf.Reset()
	p.Percent(240)
	if !strings.Contains(buf.String(), " 100%") {
		t.Errorf("Percent(240) = %q, want it clamped to 100%%", buf.String())
	}
}

func TestClearDropsBarKeepsLabel(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, enabled: true, bar: true}
	p.Stage("image")
	p.Percent(5)
	buf.Reset()
	p.Clear()
	// The bar and the percentage go away, the stage label stays.
	if got, want := buf.String(), "\x1b]9;4;0\x07\x1b]0;aigc-cli · image\x07"; got != want {
		t.Errorf("Clear = %q, want %q", got, want)
	}
}

func TestNilProgressIsNoop(t *testing.T) {
	var p *Progress
	if p.Enabled() {
		t.Error("nil Progress must report Enabled() == false")
	}
	p.Stage("x")
	p.Percent(1)
	p.Busy()
	p.Finish("x")
	p.Clear()
}

func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{name: "control chars stripped", in: "a\x1b]0;pwn\x07b\nc", max: 48, want: "a]0;pwnbc"},
		{name: "whitespace collapsed", in: "  image \t 40%  ", max: 48, want: "image 40%"},
		{name: "truncated with ellipsis", in: strings.Repeat("a", 60), max: 10, want: strings.Repeat("a", 9) + "…"},
		{name: "no limit", in: "abc", max: 0, want: "abc"},
	}
	for _, tc := range tests {
		if got := sanitizeTitle(tc.in, tc.max); got != tc.want {
			t.Errorf("%s: sanitizeTitle(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestNewWithNonTTYFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "osc")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if New(f).Enabled() {
		t.Error("a regular file must not enable OSC output")
	}
}

func TestDefaultIsCached(t *testing.T) {
	first := Default()
	if first != Default() {
		t.Error("Default() must return the same reporter across calls")
	}
	// Test processes run with a piped stderr, so the shared reporter is inert
	// and every call below stays a no-op.
	if first.Enabled() {
		t.Skip("stderr is a terminal in this environment")
	}
	first.Stage("x")
	first.Percent(1)
	first.Finish("x")
}

func TestClearOnInterruptNoopWhenDisabled(t *testing.T) {
	// Default has no terminal in tests; the call must not touch signal handling.
	ClearOnInterrupt()
}

func TestBusyStageWritesLabelAndBarOnce(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, enabled: true, bar: true}
	p.BusyStage("chat")
	want := "\x1b]9;4;3\x07" + "\x1b]0;aigc-cli · chat …\x07"
	if got := buf.String(); got != want {
		t.Errorf("BusyStage = %q, want %q", got, want)
	}
	buf.Reset()
	p.BusyStage("chat") // re-asserted every render by the chat TUI
	if buf.Len() != 0 {
		t.Errorf("repeated BusyStage wrote %q, want nothing", buf.String())
	}
}

func TestBusyTitleShowsElapsed(t *testing.T) {
	var buf bytes.Buffer
	p := &Progress{w: &buf, enabled: true}
	p.BusyStage("image")
	p.mu.Lock()
	p.busyAt = time.Now().Add(-95 * time.Second) // pretend the phase started long ago
	p.mu.Unlock()
	p.BusyStage("image")
	if got := buf.String(); !strings.Contains(got, "aigc-cli · image … 1m35s") {
		t.Errorf("busy title = %q, want elapsed time appended", got)
	}
}

func TestShortElapsed(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{in: 3 * time.Second, want: "3s"},
		{in: 65 * time.Second, want: "1m05s"},
		{in: 10 * time.Minute, want: "10m00s"},
		{in: 90 * time.Minute, want: "1h30m"},
	}
	for _, tc := range tests {
		if got := shortElapsed(tc.in); got != tc.want {
			t.Errorf("shortElapsed(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
