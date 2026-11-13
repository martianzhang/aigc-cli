package preview

import (
	"context"
	"os"
	"os/exec"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
	"golang.org/x/term"

	"github.com/martianzhang/aigc-cli/internal/argutil"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// pagerFallback is the terminal grid assumed when the size cannot be read from
// an otherwise interactive terminal. It mirrors the inline-preview fallback.
const (
	pagerFallbackCols = 80
	pagerFallbackRows = 24
)

// Pager hooks are variables so tests can drive the decision and capture the
// content without spawning less.
var (
	pagerTerminalSize = defaultPagerTerminalSize
	pagerLookPath     = exec.LookPath
	pagerRun          = runPagerProcess
)

// writePagedText writes content to stdout, running it through a pager when it
// is taller than the terminal. Paging is skipped for --no-pager, non-terminal
// output, content that fits, and when no pager is available.
func writePagedText(content string, noPager bool) {
	content = ensureTrailingNewline(content)
	if content == "" {
		return
	}
	if noPager || !pagerNeeded(content) {
		ui.Print(content)
		return
	}
	argv, ok := pagerCommand()
	if !ok {
		ui.Print(content)
		return
	}
	started, _ := pagerRun(argv, content)
	if !started {
		// The pager could not be started at all; showing the text is better
		// than dropping it.
		ui.Print(content)
	}
}

// pagerNeeded reports whether content is taller than the terminal, counting
// soft-wrapped long lines as the terminal would display them.
func pagerNeeded(content string) bool {
	cols, rows, ok := pagerTerminalSize()
	if !ok || cols <= 0 || rows <= 0 {
		return false
	}
	return visualLines(content, cols) > rows
}

// visualLines counts the rows content occupies at the given width.
func visualLines(content string, cols int) int {
	total := 0
	for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		width := xansi.StringWidth(line)
		total += max(1, (width+cols-1)/cols)
	}
	return total
}

// defaultPagerTerminalSize reads the real terminal size. A UI buffer (tests,
// MCP redirection) never pages.
func defaultPagerTerminalSize() (cols, rows int, ok bool) {
	file, isFile := ui.Out().(*os.File)
	if !isFile {
		return 0, 0, false
	}
	cols, rows, err := term.GetSize(int(file.Fd()))
	if err != nil || cols <= 0 || rows <= 0 {
		if !term.IsTerminal(int(file.Fd())) {
			return 0, 0, false
		}
		return pagerFallbackCols, pagerFallbackRows, true
	}
	return cols, rows, true
}

// pagerCommand resolves the pager: $PAGER when set, then less -R, then more.
func pagerCommand() ([]string, bool) {
	if custom := strings.TrimSpace(os.Getenv("PAGER")); custom != "" {
		if argv := argutil.Split(custom); len(argv) > 0 {
			return argv, true
		}
	}
	if path, err := pagerLookPath("less"); err == nil {
		return []string{path, "-R"}, true
	}
	if path, err := pagerLookPath("more"); err == nil {
		return []string{path}, true
	}
	return nil, false
}

// runPagerProcess writes content to a temp file and hands that file to the
// pager, leaving stdin attached to the terminal. Feeding content through stdin
// works for less (it reopens /dev/tty for keys) but not for `more` on Windows,
// where redirected stdin breaks interactive paging.
func runPagerProcess(argv []string, content string) (bool, error) {
	file, err := os.CreateTemp("", "aigc-cli-pager-*.txt")
	if err != nil {
		return false, err
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}

	args := append(append([]string{}, argv[1:]...), name)
	cmd := exec.CommandContext(context.Background(), argv[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = ui.Out()
	cmd.Stderr = ui.Err()
	if os.Getenv("LESS") == "" {
		// F: quit when it fits, R: keep ANSI colours, X: leave the text in the
		// scrollback instead of switching to the alternate screen.
		cmd.Env = append(os.Environ(), "LESS=FRX")
	}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	return true, cmd.Wait()
}

// ensureTrailingNewline keeps the shell prompt from landing on the last line.
func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
