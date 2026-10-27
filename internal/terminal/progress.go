// Package terminal emits the OSC escape sequences that let the host terminal
// show the running command's progress in its tab/window title and, where the
// terminal supports it, in a native progress indicator.
//
// The sequences are advisory: terminals that do not understand them ignore them,
// and every write is suppressed when the target stream is not a terminal or when
// AIGC_CLI_NO_TITLE is set, so piped/CI output stays byte-for-byte unchanged.
package terminal

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
)

const (
	// EnvDisable turns every title/progress write off (e.g. for shells that
	// rename their tabs themselves).
	EnvDisable = "AIGC_CLI_NO_TITLE"
	// EnvTitleBar forces the indeterminate/determinate OSC 9;4 progress bar on
	// for terminals we do not recognize. Terminals without support ignore it.
	EnvTitleBar = "AIGC_CLI_TITLE_BAR"
	// EnvTitle forces title writes on for a terminal we do not recognize.
	EnvTitle = "AIGC_CLI_TITLE"

	// seqTitle sets the icon+window title (xterm OSC 0; iTerm2 shows it in the tab).
	seqTitle = "\x1b]0;%s\x07"
	// seqProgressPercent paints a determinate bar (OSC 9;4 state 1).
	seqProgressPercent = "\x1b]9;4;1;%d\x07"
	// seqProgressBusy paints an indeterminate bar (OSC 9;4 state 3).
	seqProgressBusy = "\x1b]9;4;3\x07"
	// seqProgressClear removes the bar (OSC 9;4 state 0).
	seqProgressClear = "\x1b]9;4;0\x07"

	titlePrefix   = "aigc-cli"
	titleMaxRunes = 48
	// keepAliveTick re-sends the progress bar so terminals that expire an idle
	// indicator keep showing it during long silent phases (polling, inference).
	keepAliveTick = time.Second
)

// Progress renders a one-line status into the terminal title, e.g.
//
//	aigc-cli · video 45%
//
// and mirrors the percentage into the tab's native progress indicator on
// supporting terminals (iTerm2, Windows Terminal, WezTerm, Ghostty, kitty).
// The zero value is disabled, and every method is nil-safe, so callers may hold
// an optional *Progress and call it unconditionally.
type Progress struct {
	mu        sync.Mutex
	w         io.Writer
	enabled   bool
	bar       bool
	stage     string
	pct       int
	busy      bool
	stop      chan struct{}
	lastTitle string
	lastBar   string
	barActive bool
	busyAt    time.Time
}

// New returns a Progress writing to w. It stays disabled unless w is a terminal
// with ANSI escape processing enabled (on Windows that means Windows Terminal,
// or a console whose VT mode could be switched on) and AIGC_CLI_NO_TITLE is unset.
func New(w io.Writer) *Progress {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return &Progress{}
	}
	if !enableVirtualTerminal(f) {
		return &Progress{}
	}
	return newEnabled(w, os.Getenv)
}

// newEnabled builds an enabled Progress; env is injected so detection is testable.
func newEnabled(w io.Writer, getenv func(string) string) *Progress {
	if getenv(EnvDisable) != "" || !supportsTitle(getenv) {
		return &Progress{}
	}
	return &Progress{w: w, enabled: true, bar: supportsProgressBar(getenv)}
}

// titleTERMPrefixes are the $TERM families whose parsers implement OSC 0. An
// unrecognized $TERM is treated as "no titles" on purpose: the Linux VT
// (TERM=linux) and other line-mode consoles would echo the escape bytes as
// visible garbage, and losing a title is always better than corrupting output.
var titleTERMPrefixes = []string{
	"xterm", "screen", "tmux", "rxvt", "alacritty", "wezterm", "kitty",
	"ghostty", "foot", "konsole", "vte", "cygwin", "msys", "st-", "eterm",
}

// supportsTitle reports whether writing OSC 0 to this terminal is safe.
func supportsTitle(getenv func(string) string) bool {
	if getenv(EnvTitle) == "1" {
		return true
	}
	termName := getenv("TERM")
	for _, prefix := range titleTERMPrefixes {
		if strings.HasPrefix(termName, prefix) {
			return true
		}
	}
	switch getenv("TERM_PROGRAM") {
	case "iTerm.app", "Apple_Terminal", "WezTerm", "ghostty", "vscode", "Hyper", "Tabby", "mintty":
		return true
	}
	if strings.Contains(getenv("LC_TERMINAL"), "iTerm") {
		return true
	}
	// Windows consoles (Windows Terminal, conhost) set no $TERM; ANSI support
	// there is proven separately by enableVirtualTerminal before we get here.
	return termName == ""
}

var (
	defaultOnce sync.Once
	defaultP    *Progress
)

// Default returns the process-wide reporter, writing to stderr: stdout must stay
// machine-readable (JSON output, MCP's stdio protocol). It is disabled when
// stderr is not a terminal.
func Default() *Progress {
	defaultOnce.Do(func() {
		defaultP = New(os.Stderr)
	})
	return defaultP
}

// ClearOnInterrupt clears the tab indicator when Ctrl-C arrives, then re-raises
// SIGINT so the exit status stays what the shell expects. Without this, a
// signal-killed process would leave a stale progress bar behind (deferred
// cleanup never runs). It is a no-op when the reporter is disabled, so plain
// pipes and CI keep their default signal behavior.
func ClearOnInterrupt() {
	p := Default()
	if !p.Enabled() {
		return
	}
	clearOnInterrupt(p, reRaiseInterrupt)
}

// clearOnInterrupt installs the handler. reRaise redelivers the signal once the
// handler is removed; it is injected so tests can observe the cleanup without
// terminating the test process.
func clearOnInterrupt(p *Progress, reRaise func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		<-ch
		p.Finish("")
		// Dropping the last handler restores the default disposition.
		signal.Stop(ch)
		reRaise()
	}()
}

// Enabled reports whether any sequence will be written at all.
func (p *Progress) Enabled() bool {
	return p != nil && p.enabled
}

// Stage labels the current phase, e.g. "image", "video", "download".
func (p *Progress) Stage(stage string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stage = stage
	p.emitTitle()
}

// StageIfEmpty labels the phase only when nothing is labelled yet, so a
// command-level stage ("image") survives a lower-level helper ("task").
func (p *Progress) StageIfEmpty(stage string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stage != "" {
		return
	}
	p.stage = stage
	p.emitTitle()
}

// Percent reports determinate progress; values outside 0-100 are clamped.
func (p *Progress) Percent(pct int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pct = max(0, min(100, pct))
	p.busy = false
	p.busyAt = time.Time{}
	p.emitBar(false)
	p.emitTitle()
	p.armKeepAlive()
}

// Busy shows an indeterminate indicator for work with no percentage.
func (p *Progress) Busy() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pct = 0
	p.markBusy()
	p.emitBar(false)
	p.emitTitle()
	p.armKeepAlive()
}

// BusyStage labels the phase and marks it indeterminate in one write, so
// callers that re-assert their state on every render stay silent. It replaces
// Stage+Busy for the chat TUI, where two calls would flip-flop the title.
func (p *Progress) BusyStage(stage string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stage != stage || !p.busy {
		p.stage = stage
		p.markBusy()
	}
	p.emitBar(false)
	p.emitTitle()
	p.armKeepAlive()
}

// markBusy enters the indeterminate state and restarts the elapsed clock when
// the phase is new; the caller holds p.mu.
func (p *Progress) markBusy() {
	p.busy = true
	p.pct = 0
	p.busyAt = time.Now()
}

// Finish clears the progress bar and settles the title on summary (or on the
// bare program name when summary is empty). Safe to call more than once.
func (p *Progress) Finish(summary string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.disarmKeepAlive()
	p.clearBar()
	p.stage = summary
	p.pct = 0
	p.busy = false
	p.busyAt = time.Time{}
	p.emitTitle()
}

// shortElapsed renders a compact duration for the busy title: 12s, 1m05s, 10m.
func shortElapsed(d time.Duration) string {
	secs := int(d.Round(time.Second).Seconds())
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds", secs)
	case secs < 3600:
		return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
	default:
		return fmt.Sprintf("%dh%02dm", secs/3600, (secs%3600)/60)
	}
}

// Clear removes the progress bar and drops the percentage but keeps the stage
// label: the command is still running, it just has nothing to measure.
func (p *Progress) Clear() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.disarmKeepAlive()
	p.clearBar()
	p.pct = 0
	p.busy = false
	p.busyAt = time.Time{}
	p.emitTitle()
}

// emitTitle writes the composed title unless it is unchanged — long-running
// callers (the chat TUI) re-assert it on every render; the caller holds p.mu.
func (p *Progress) emitTitle() {
	if !p.enabled {
		return
	}
	title := titlePrefix
	if p.stage != "" {
		title += " · " + p.stage
	}
	switch {
	case p.busy:
		title += " …"
		if !p.busyAt.IsZero() {
			if d := time.Since(p.busyAt); d >= time.Second {
				title += " " + shortElapsed(d)
			}
		}
	case p.pct > 0:
		title += fmt.Sprintf(" %d%%", p.pct)
	}
	title = sanitizeTitle(title, titleMaxRunes)
	if title == p.lastTitle {
		return
	}
	p.lastTitle = title
	p.write(fmt.Sprintf(seqTitle, title))
}

// emitBar writes the progress indicator unless it is unchanged (keep-alive
// re-sends bypass the check); the caller holds p.mu.
func (p *Progress) emitBar(force bool) {
	if !p.bar {
		return
	}
	seq := seqProgressBusy
	if !p.busy {
		seq = fmt.Sprintf(seqProgressPercent, p.pct)
	}
	if !force && seq == p.lastBar {
		return
	}
	p.lastBar = seq
	p.barActive = true
	p.write(seq)
}

// clearBar removes the indicator once, so repeated Finish/Clear calls (the chat
// TUI calls Finish on every idle render) stay silent; the caller holds p.mu.
func (p *Progress) clearBar() {
	if !p.barActive {
		return
	}
	p.barActive = false
	p.lastBar = ""
	p.write(seqProgressClear)
}

func (p *Progress) write(seq string) {
	if !p.enabled || p.w == nil {
		return
	}
	_, _ = io.WriteString(p.w, seq)
}

// armKeepAlive re-sends the indicator every second until Finish/Clear; the
// caller holds p.mu.
func (p *Progress) armKeepAlive() {
	if !p.bar || p.stop != nil {
		return
	}
	stop := make(chan struct{})
	p.stop = stop
	go func() {
		t := time.NewTicker(keepAliveTick)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				p.mu.Lock()
				p.emitBar(true)
				p.emitTitle() // refreshes the elapsed counter in a busy title
				p.mu.Unlock()
			}
		}
	}()
}

// disarmKeepAlive stops the keep-alive goroutine; the caller holds p.mu.
func (p *Progress) disarmKeepAlive() {
	if p.stop == nil {
		return
	}
	close(p.stop)
	p.stop = nil
}

// supportsProgressBar reports whether the terminal understands OSC 9;4. Unknown
// terminals simply get the title-only experience (which every modern terminal
// and PowerShell/Windows Terminal honour via OSC 0).
func supportsProgressBar(getenv func(string) string) bool {
	if getenv(EnvTitleBar) == "1" {
		return true
	}
	if getenv("WT_SESSION") != "" { // Windows Terminal (also hosts PowerShell)
		return true
	}
	if getenv("TMUX") != "" { // tmux drops OSC 9;4 without allow-passthrough
		return false
	}
	if strings.HasPrefix(getenv("TERM"), "xterm-kitty") {
		return true
	}
	switch getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "ghostty", "vscode":
		return true
	}
	return strings.Contains(getenv("LC_TERMINAL"), "iTerm")
}

// sanitizeTitle drops control characters — an OSC payload ends at BEL, so a raw
// newline or ESC would terminate the sequence early — and caps the length.
func sanitizeTitle(s string, maxRunes int) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if maxRunes > 0 && utf8.RuneCountInString(cleaned) > maxRunes {
		runes := []rune(cleaned)
		cleaned = string(runes[:maxRunes-1]) + "…"
	}
	return cleaned
}
