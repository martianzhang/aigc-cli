package web

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/martianzhang/aigc-cli/internal/terminal"
)

// mediaProgress renders a single-line download progress on a terminal and a
// final size summary on every stream. stdout stays machine-readable (the saved
// path); progress goes to stderr.
type mediaProgress struct {
	w       io.Writer
	tty     bool
	start   time.Time
	lineLen int
	prog    *terminal.Progress
}

func newMediaProgress(w io.Writer) *mediaProgress {
	tty := false
	if f, ok := w.(*os.File); ok {
		tty = term.IsTerminal(int(f.Fd()))
	}
	prog := terminal.Default()
	prog.Stage("download")
	return &mediaProgress{w: w, tty: tty, start: time.Now(), prog: prog}
}

// report mirrors the byte progress into the terminal title/tab indicator. It
// runs for non-TTY streams too; the reporter itself is a no-op when disabled.
func (p *mediaProgress) report(done, total int64) {
	if p == nil || p.prog == nil {
		return
	}
	if total > 0 {
		p.prog.Percent(int(min(done, total) * 100 / total))
		return
	}
	p.prog.Busy()
}

// Update redraws the progress line at most as often as saveMedia calls it.
func (p *mediaProgress) Update(done, total int64) {
	if p == nil {
		return
	}
	p.report(done, total)
	if !p.tty || p.w == nil {
		return
	}
	elapsed := time.Since(p.start)
	if elapsed <= 0 {
		elapsed = time.Millisecond
	}
	speed := float64(done) / elapsed.Seconds()

	var b strings.Builder
	b.WriteString(formatByteSize(done))
	if total > 0 {
		fmt.Fprintf(&b, " / %s  %d%%", formatByteSize(total), done*100/total)
	}
	fmt.Fprintf(&b, "  %s/s", formatByteSize(int64(speed)))
	if total > 0 && speed > 0 && done < total {
		remaining := time.Duration(float64(total-done) / speed * float64(time.Second))
		fmt.Fprintf(&b, "  ETA %s", formatETA(remaining))
	}

	line := b.String()
	pad := ""
	if p.lineLen > len(line) {
		pad = strings.Repeat(" ", p.lineLen-len(line))
	}
	fmt.Fprint(p.w, "\r"+line+pad)
	p.lineLen = len(line)
}

// Finish clears the live line and prints the final size/speed summary. It also
// runs for non-TTY streams, so redirected/piped runs still report the size.
func (p *mediaProgress) Finish(done int64) {
	if p == nil || p.w == nil {
		return
	}
	if p.prog != nil {
		p.prog.Finish("download complete")
	}
	if p.tty && p.lineLen > 0 {
		fmt.Fprint(p.w, "\r"+strings.Repeat(" ", p.lineLen)+"\r")
	}
	elapsed := time.Since(p.start)
	if elapsed <= 0 {
		elapsed = time.Millisecond
	}
	speed := float64(done) / elapsed.Seconds()
	fmt.Fprintf(p.w, "downloaded %s in %s (%s/s)\n",
		formatByteSize(done), formatElapsed(elapsed), formatByteSize(int64(speed)))
}

// Abort clears a partially drawn progress line after an error.
func (p *mediaProgress) Abort() {
	if p == nil {
		return
	}
	if p.prog != nil {
		p.prog.Finish("download aborted")
	}
	if !p.tty || p.w == nil || p.lineLen == 0 {
		return
	}
	fmt.Fprint(p.w, "\r"+strings.Repeat(" ", p.lineLen)+"\r")
	p.lineLen = 0
}

func formatByteSize(n int64) string {
	if n < 0 {
		n = 0
	}
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func formatETA(d time.Duration) string {
	if d < 0 {
		return "--:--"
	}
	secs := int(d.Round(time.Second).Seconds())
	if secs >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", secs/3600, (secs%3600)/60, secs%60)
	}
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	secs := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
}
