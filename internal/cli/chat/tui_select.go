package chat

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// copyToClipboard is a variable so tests can capture the copied text without
// touching the system clipboard.
var copyToClipboard = clipboard.WriteAll

// selectable transcript region.
//
// The terminal's own selection copies raw screen cells, which means every line
// of a message drags its "┃ " bar and padding along. Like OpenTUI (the renderer
// behind OpenCode), the TUI therefore implements selection itself: a drag marks
// whole transcript rows, and the text is rebuilt from the rendered lines with
// the decoration stripped — borders and padding are never part of what is
// copied.

// startSelection begins a mouse selection at screen row y.
func (m *chatModel) startSelection(msg tea.MouseMsg) {
	line, ok := m.transcriptLineAt(msg.Y)
	if !ok {
		return
	}
	m.selActive = true
	m.selAnchor, m.selFocus = line, line
	m.selMoved = false
	m.viewport.SetContent(m.selectionView())
}

// dragSelection extends the active selection to screen row y. Rows above or
// below the rendered transcript clamp to its first/last line, so dragging past
// the end still selects everything up to that edge.
func (m *chatModel) dragSelection(msg tea.MouseMsg) {
	if !m.selActive {
		return
	}
	line, ok := m.dragLineAt(msg.Y)
	if !ok {
		return
	}
	if line != m.selFocus {
		m.selFocus = line
		m.selMoved = m.selMoved || line != m.selAnchor
	}
	m.viewport.SetContent(m.selectionView())
}

// dragLineAt maps a drag position to a transcript line, clamping to the visible
// rows and to the first/last transcript line.
func (m *chatModel) dragLineAt(y int) (int, bool) {
	total := m.transcriptLines()
	if total == 0 || m.viewport.Height <= 0 {
		return 0, false
	}
	row := y - 1
	row = max(row, 0)
	row = min(row, m.viewport.Height-1)
	line := m.viewport.YOffset + row
	return min(line, total-1), true
}

// finishSelection copies the selected rows (if the drag actually covered
// something) and restores the plain transcript.
func (m *chatModel) finishSelection() {
	if !m.selActive {
		return
	}
	active, moved := m.selActive, m.selMoved
	text := m.selectedText()
	m.selActive = false
	m.selMoved = false

	if active && moved && text != "" {
		lines := strings.Count(text, "\n") + 1
		if err := copyToClipboard(text); err != nil {
			m.messages = append(m.messages, message{role: "system", content: fmt.Sprintf("Copy failed: %v", err)})
		} else {
			m.messages = append(m.messages, message{
				role:    "system",
				content: fmt.Sprintf("✓ Copied %d line(s) to clipboard", lines),
			})
		}
	}
	m.refreshViewport()
}

// clearSelection drops any selection without copying.
func (m *chatModel) clearSelection() {
	if !m.selActive {
		return
	}
	m.selActive = false
	m.selMoved = false
	m.refreshViewport()
}

// transcriptLineAt maps a screen row to a transcript line index. The header
// occupies the first row, so the viewport starts at row 1.
func (m *chatModel) transcriptLineAt(y int) (int, bool) {
	row := y - 1
	if row < 0 || row >= m.viewport.Height {
		return 0, false
	}
	line := m.viewport.YOffset + row
	if line < 0 || line >= m.transcriptLines() {
		return 0, false
	}
	return line, true
}

// transcriptLines is the number of rendered transcript rows.
func (m *chatModel) transcriptLines() int {
	if m.msgCache == "" {
		return 0
	}
	return strings.Count(m.msgCache, "\n") + 1
}

// selectionBounds returns the inclusive line range of the active selection.
func (m *chatModel) selectionBounds() (int, int) {
	lo, hi := m.selAnchor, m.selFocus
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= m.transcriptLines() {
		hi = m.transcriptLines() - 1
	}
	return lo, hi
}

// selectionView renders the transcript with the selected rows highlighted.
func (m *chatModel) selectionView() string {
	if !m.selActive {
		return m.msgCache
	}
	lines := strings.Split(m.msgCache, "\n")
	lo, hi := m.selectionBounds()
	width := max(m.viewport.Width-2, 10)
	for i := lo; i <= hi && i < len(lines); i++ {
		lines[i] = reverseVideoRow(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

// msgRange maps rendered transcript lines to the message that produced them.
// seps[i] is the separator to use before line start+i when rebuilding text
// ("" for the first line, then "\n", " " or "").
type msgRange struct {
	role  string
	start int
	end   int // exclusive
	seps  []string
}

// rangeFor returns the message range containing a transcript line. Blank
// separator lines and the streaming preview fall outside every range and get a
// zero value (role ""), which still has its bar stripped.
func (m *chatModel) rangeFor(line int) msgRange {
	for _, r := range m.msgRanges {
		if line >= r.start && line < r.end {
			return r
		}
	}
	return msgRange{}
}

// selectedText rebuilds the selected rows from the rendered text: ANSI styles,
// the block frame ("┃ " for messages, a two-space indent for tool output) and
// trailing padding are dropped, and rows that were only wrapped for display are
// joined back together (see wrapANSI), so pasting yields the content only.
func (m *chatModel) selectedText() string {
	if m.transcriptLines() == 0 {
		return ""
	}
	lines := strings.Split(m.msgCache, "\n")
	lo, hi := m.selectionBounds()
	var b strings.Builder
	for i := lo; i <= hi && i < len(lines); i++ {
		if i > lo {
			b.WriteString(m.joinBefore(i))
		}
		r := m.rangeFor(i)
		b.WriteString(cleanTranscriptLine(r.role, i-r.start, lines[i]))
	}
	return strings.TrimRight(b.String(), "\n")
}

// joinBefore returns the separator that belongs before transcript line i: a
// real newline at message boundaries (and for rows without wrap metadata), or
// the soft-wrap separator recorded while rendering.
func (m *chatModel) joinBefore(line int) string {
	r := m.rangeFor(line)
	idx := line - r.start
	if r.seps == nil || idx <= 0 || idx >= len(r.seps) {
		return "\n"
	}
	return r.seps[idx]
}

// cleanTranscriptLine strips the decoration from one rendered transcript row.
// rel is the row's offset inside its message: tool cards draw the header with
// the "┃ " bar and every following row with a two-space indent.
func cleanTranscriptLine(role string, rel int, line string) string {
	s := ansi.Strip(line)
	if role == "tool" && rel > 0 {
		s = strings.TrimPrefix(s, "  ")
	} else if rest, ok := strings.CutPrefix(s, "┃"); ok {
		s = strings.TrimPrefix(rest, " ")
	}
	return strings.TrimRight(s, " ")
}

// reverseVideoRow highlights one row in place. Inner SGR resets would cancel
// the reverse attribute, so each reset re-applies it.
func reverseVideoRow(line string, width int) string {
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m\x1b[7m")
	return "\x1b[7m" + line + "\x1b[0m"
}
