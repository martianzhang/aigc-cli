package chat

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// withClipboardCapture replaces the clipboard writer for the duration of a test
// and returns a pointer to the last copied text.
func withClipboardCapture(t *testing.T) *string {
	t.Helper()
	var copied string
	orig := copyToClipboard
	copyToClipboard = func(s string) error {
		copied = s
		return nil
	}
	t.Cleanup(func() { copyToClipboard = orig })
	return &copied
}

// selectionTestModel returns a model whose transcript is two short messages.
func selectionTestModel(t *testing.T) *chatModel {
	t.Helper()
	m := newTestModel(t)
	m.messages = []message{
		{role: "user", content: "hello"},
		{role: "assistant", content: "line one\nline two"},
	}
	m.refreshViewport()
	return m
}

func TestCleanTranscriptLine(t *testing.T) {
	cases := []struct {
		role string
		rel  int
		in   string
		want string
	}{
		{"user", 0, "\x1b[38;5;75m┃\x1b[0m hello   ", "hello"},
		{"user", 1, "┃ hello", "hello"},
		{"assistant", 2, "┃", ""},
		{"assistant", 1, "┃ • item", "• item"},
		{"tool", 0, "┃ 🛠️  shell", "🛠️  shell"},
		{"tool", 1, "  $ ls", "$ ls"},
		{"tool", 2, "    nested/file", "  nested/file"},
		{"system", 0, "  plain text", "  plain text"},
	}
	for _, tc := range cases {
		if got := cleanTranscriptLine(tc.role, tc.rel, tc.in); got != tc.want {
			t.Errorf("cleanTranscriptLine(%q, %d, %q) = %q, want %q", tc.role, tc.rel, tc.in, got, tc.want)
		}
	}
}

// TestSelectedTextStripsToolIndent guards that !shell output (a tool card with
// a two-space body indent) copies without the indentation.
func TestSelectedTextStripsToolIndent(t *testing.T) {
	m := newTestModel(t)
	m.messages = []message{
		{role: "tool", tool: "shell", content: "$ ls\nfile1\n  nested/file2"},
	}
	m.refreshViewport()
	m.selActive = true
	m.selAnchor, m.selFocus = 0, m.transcriptLines()-1

	got := m.selectedText()
	want := "🛠️  shell\n$ ls\nfile1\n  nested/file2"
	if got != want {
		t.Errorf("copied tool output = %q, want %q", got, want)
	}
}

// TestMessageRangesAlignWithRenderedLines guards the line bookkeeping that
// copy-on-select relies on: each range must start at its message's first
// rendered line (and the separator line between messages must not shift them).
func TestMessageRangesAlignWithRenderedLines(t *testing.T) {
	m := newTestModel(t) // banner first
	m.messages = append(m.messages, message{role: "user", content: "hi"})
	m.messages = append(m.messages, message{role: "tool", tool: "shell", content: "$ ls\nfile1"})
	m.refreshViewport()

	lines := strings.Split(m.msgCache, "\n")
	if len(m.msgRanges) != len(m.messages) {
		t.Fatalf("got %d ranges, want %d", len(m.msgRanges), len(m.messages))
	}
	for i, r := range m.msgRanges {
		if r.start < 0 || r.end > len(lines) || r.start >= r.end {
			t.Errorf("range %d = %+v outside %d rendered lines", i, r, len(lines))
		}
	}
	user := m.msgRanges[1]
	if got := stripANSI(lines[user.start]); !strings.HasPrefix(got, "┃ You:") {
		t.Errorf("user range starts at %q, want the You: line", got)
	}
	tool := m.msgRanges[2]
	if got := stripANSI(lines[tool.start]); !strings.Contains(got, "🛠️  shell") {
		t.Errorf("tool range starts at %q, want the tool header", got)
	}
	if got := stripANSI(lines[tool.start+1]); !strings.Contains(got, "$ ls") {
		t.Errorf("tool body line = %q, want the first output row", got)
	}
}

// TestCopiedTextRejoinsWrappedProse guards that display wrapping is not turned
// into hard newlines when copying: every paragraph must come back as one line.
func TestCopiedTextRejoinsWrappedProse(t *testing.T) {
	zh := strings.TrimSpace(strings.Repeat("这是第一段很长的中文内容。", 8))
	en := strings.TrimSpace(strings.Repeat("This is a long English paragraph that must wrap. ", 4))
	m := newTestModel(t)
	m.messages = []message{{role: "assistant", content: zh + "\n\n" + en}}
	m.refreshViewport()

	if lines := m.transcriptLines(); lines < 5 {
		t.Fatalf("test setup did not wrap: %d transcript lines", lines)
	}
	m.selActive = true
	m.selAnchor, m.selFocus = 0, m.transcriptLines()-1

	got := m.selectedText()
	want := "Assistant:\n" + zh + "\n\n" + en
	if got != want {
		t.Errorf("copied text = %q\nwant %q", got, want)
	}
}

// TestCopiedMarkdownRejoinsWrappedCJK guards the markdown path: a long Chinese
// paragraph (which glamour cannot wrap) must copy as a single line.
func TestCopiedMarkdownRejoinsWrappedCJK(t *testing.T) {
	zh := strings.TrimSpace(strings.Repeat("这是第一段很长的中文内容，用来观察换行后的复制效果。", 4))
	m := newTestModel(t)
	m.messages = []message{{role: "assistant", content: "**要点：**" + zh}}
	m.refreshViewport()

	m.selActive = true
	m.selAnchor, m.selFocus = 0, m.transcriptLines()-1
	got := m.selectedText()

	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("copied text has %d lines, want label + one paragraph: %q", len(lines), got)
	}
	if !strings.Contains(lines[1], zh) {
		t.Errorf("copied paragraph = %q, want it to contain %q", lines[1], zh)
	}
}

// TestCopiedTextKeepsHardBreaks guards that real newlines in the source survive
// a copy (only display wrapping is undone).
func TestCopiedTextKeepsHardBreaks(t *testing.T) {
	m := newTestModel(t)
	m.messages = []message{{role: "assistant", content: "line one\nline two\n\nline four"}}
	m.refreshViewport()

	m.selActive = true
	m.selAnchor, m.selFocus = 0, m.transcriptLines()-1

	got := m.selectedText()
	want := "Assistant:\nline one\nline two\n\nline four"
	if got != want {
		t.Errorf("copied text = %q, want %q", got, want)
	}
}

// TestRenderMarkdownTableFitsWidth guards that a markdown table is not blown up
// to the wide render limit used for prose.
func TestRenderMarkdownTableFitsWidth(t *testing.T) {
	m := newTestModel(t)
	m.messages = []message{{role: "assistant", content: "Intro.\n\n| A | B |\n|---|---|\n| one | two |"}}
	m.refreshViewport()

	for _, line := range strings.Split(m.msgCache, "\n") {
		if w := lipgloss.Width(strings.TrimRight(stripANSI(line), " ")); w > m.bodyWidth()+2 {
			t.Errorf("table line width = %d, want <= %d: %q", w, m.bodyWidth()+2, stripANSI(line))
		}
	}
}

// TestSelectedTextStripsDecoration guards the whole point: copied rows carry no
// message bar, no bar padding and no trailing blanks.
func TestSelectedTextStripsDecoration(t *testing.T) {
	m := selectionTestModel(t)
	m.selActive = true
	m.selAnchor, m.selFocus = 0, m.transcriptLines()-1

	got := m.selectedText()
	if strings.Contains(got, "┃") {
		t.Errorf("copied text still contains the message bar:\n%q", got)
	}
	if strings.Contains(got, " \n") || strings.HasSuffix(got, " ") {
		t.Errorf("copied text has trailing padding: %q", got)
	}
	want := "You:\nhello\nAssistant:\nline one\nline two"
	if got != want {
		t.Errorf("copied text = %q, want %q", got, want)
	}
}

// TestDragSelectionCopiesCleanText drives the mouse path end to end.
func TestDragSelectionCopiesCleanText(t *testing.T) {
	copied := withClipboardCapture(t)
	m := selectionTestModel(t)

	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: 1})
	m = updated.(*chatModel)
	if !m.selActive {
		t.Fatal("press on the first transcript row did not start a selection")
	}

	updated, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, Y: 2})
	m = updated.(*chatModel)
	if view := m.selectionView(); !strings.Contains(view, "\x1b[7m") {
		t.Errorf("selection is not highlighted:\n%q", view)
	}

	updated, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Y: 2})
	m = updated.(*chatModel)
	if *copied != "You:\nhello" {
		t.Errorf("copied = %q, want %q", *copied, "You:\nhello")
	}
	if m.selActive {
		t.Error("selection still active after release")
	}
	if got := m.messages[len(m.messages)-1]; !strings.Contains(got.content, "Copied 2 line") {
		t.Errorf("no copy confirmation: %+v", got)
	}
}

// TestKeyPressClearsStaleSelection guards that a lost mouse release cannot
// leave a highlighted selection behind.
func TestKeyPressClearsStaleSelection(t *testing.T) {
	m := selectionTestModel(t)
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: 1})
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, Y: 2})
	if !m.selActive {
		t.Fatal("selection did not start")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if m.selActive {
		t.Error("selection survived a key press")
	}
	if strings.Contains(m.selectionView(), "\x1b[7m") {
		t.Error("selection highlight survived a key press")
	}
}

// TestDragPastEndClampsToLastLine guards that dragging below the transcript
// still selects everything up to its end.
func TestDragPastEndClampsToLastLine(t *testing.T) {
	copied := withClipboardCapture(t)
	m := selectionTestModel(t)

	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: 1})
	m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, Y: 200})
	m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Y: 200})

	want := "You:\nhello\nAssistant:\nline one\nline two"
	if *copied != want {
		t.Errorf("copied = %q, want %q", *copied, want)
	}
}

// TestClickDoesNotCopy guards that a plain click does not write to the
// clipboard.
func TestClickDoesNotCopy(t *testing.T) {
	copied := withClipboardCapture(t)
	m := selectionTestModel(t)

	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: 1})
	m = updated.(*chatModel)
	updated, _ = m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Y: 1})
	m = updated.(*chatModel)

	if *copied != "" {
		t.Errorf("a click copied %q", *copied)
	}
	if m.selActive {
		t.Error("selection still active after click")
	}
}

// TestSelectionIgnoresHeaderRow guards that dragging on the header does not
// start a selection.
func TestSelectionIgnoresHeaderRow(t *testing.T) {
	m := selectionTestModel(t)
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: 0})
	m = updated.(*chatModel)
	if m.selActive {
		t.Error("selection started on the header row")
	}
}

// TestWheelStillScrolls guards that the selection handling did not break the
// wheel path.
func TestWheelStillScrolls(t *testing.T) {
	m := newTestModel(t)
	m.messages = []message{{role: "assistant", content: strings.Repeat("line\n", 200)}}
	m.refreshViewport()
	m.viewport.GotoBottom()
	before := m.viewport.YOffset

	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	m = updated.(*chatModel)
	if m.viewport.YOffset >= before {
		t.Errorf("wheel up did not scroll: before=%d after=%d", before, m.viewport.YOffset)
	}
}
