package chat

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/martianzhang/aigc-cli/internal/types"
)

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiEscape.ReplaceAllString(s, "") }

// TestRenderMessageBlockFullHeightBar guards that the coloured left bar runs
// down every line of a message, not just the role label, and that multi-line
// bodies keep the same indent.
func TestRenderMessageBlockFullHeightBar(t *testing.T) {
	m := newTestModel(t)
	cases := []struct{ name, role, label string }{
		{"user", "user", "You:"},
		{"assistant", "assistant", "Assistant:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := strings.Split(stripANSI(m.renderOneMessage(message{
				role:    tc.role,
				content: "第一行\n第二行",
			})), "\n")

			want := []string{"┃ " + tc.label, "┃ 第一行", "┃ 第二行"}
			if len(lines) != len(want) {
				t.Fatalf("got %d lines, want %d: %q", len(lines), len(want), lines)
			}
			for i := range want {
				if strings.TrimRight(lines[i], " ") != want[i] {
					t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
				}
			}
		})
	}
}

// TestRenderOneMessageTrimsBlankLines guards that newlines streamed at the
// start or end of a reply do not show up as empty rows.
func TestRenderOneMessageTrimsBlankLines(t *testing.T) {
	m := newTestModel(t)
	cases := []struct {
		name, role, content, want string
	}{
		{"assistant leading and trailing", "assistant", "\n\n答案是 42。\n\n", "┃ Assistant:\n┃ 答案是 42。"},
		{"assistant trailing newline", "assistant", "答案是 42。\n", "┃ Assistant:\n┃ 答案是 42。"},
		{"user trailing newline", "user", "问题？\n", "┃ You:\n┃ 问题？"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripANSI(m.renderOneMessage(message{role: tc.role, content: tc.content}))
			lines := strings.Split(got, "\n")
			for i := range lines {
				lines[i] = strings.TrimRight(lines[i], " ")
			}
			if joined := strings.Join(lines, "\n"); joined != tc.want {
				t.Errorf("rendered = %q, want %q", joined, tc.want)
			}
		})
	}
}

// TestAgentDoneTrimsStreamedWhitespace guards the flush path: blank lines at
// the edges of the accumulated stream must not reach the stored message.
func TestAgentDoneTrimsStreamedWhitespace(t *testing.T) {
	m := newTestModel(t)
	m.turnID = 3

	updated, _ := m.Update(streamChunk{turn: 3, text: "\n\n答案是 42。\n\n"})
	m = updated.(*chatModel)
	updated, _ = m.Update(agentDone{turn: 3, history: []types.ChatMessage{{Role: "assistant", Content: "答案是 42。"}}})
	m = updated.(*chatModel)

	last := m.messages[len(m.messages)-1]
	if last.role != "assistant" {
		t.Fatalf("last message role = %q, want assistant", last.role)
	}
	if last.content != "答案是 42。" {
		t.Errorf("assistant content = %q, want %q", last.content, "答案是 42。")
	}
	if strings.Contains(m.msgCache, "\n\n\n") {
		t.Errorf("rendered transcript has extra blank lines:\n%s", m.msgCache)
	}
}

// TestStreamPreviewUsesMessageBlock guards that the live preview has the same
// label and left bar as the finished message, so nothing jumps when the turn
// ends.
func TestStreamPreviewUsesMessageBlock(t *testing.T) {
	m := newTestModel(t)
	m.turnID = 1

	updated, _ := m.Update(streamChunk{turn: 1, text: "\n\n正在生成"})
	m = updated.(*chatModel)

	lines := strings.Split(stripANSI(m.renderMessagesWithStream()), "\n")
	tail := lines[len(lines)-2:]
	want := []string{"┃ Assistant:", "┃ 正在生成"}
	for i := range want {
		if strings.TrimRight(tail[i], " ") != want[i] {
			t.Errorf("preview line %d = %q, want %q", i, tail[i], want[i])
		}
	}
}

// TestRenderMessageBlockEmptyBody guards that an empty reply (or one that was
// only whitespace) renders just the label, without a stray blank row.
func TestRenderMessageBlockEmptyBody(t *testing.T) {
	m := newTestModel(t)
	got := stripANSI(m.renderOneMessage(message{role: "assistant", content: "  \n"}))
	if got != "┃ Assistant:" {
		t.Errorf("rendered = %q, want %q", got, "┃ Assistant:")
	}
}

// TestStreamPreviewWithoutCachedMessages guards the first turn after a clear:
// with an empty transcript the preview must not start with a blank line.
func TestStreamPreviewWithoutCachedMessages(t *testing.T) {
	m := newTestModel(t)
	m.messages = nil
	m.refreshViewport()
	m.streamBuf.WriteString("\n答案")

	got := stripANSI(m.renderMessagesWithStream())
	lines := strings.Split(got, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	if got := strings.Join(lines, "\n"); got != "┃ Assistant:\n┃ 答案" {
		t.Errorf("rendered = %q, want %q", got, "┃ Assistant:\n┃ 答案")
	}
}

// TestWelcomeBannerStaysInTranscript guards that the logo/shortcut banner is
// not thrown away when the first message is sent.
func TestWelcomeBannerStaysInTranscript(t *testing.T) {
	m := newTestModel(t)
	m.appendUserMessage("你好", nil)
	m.refreshViewport()

	if len(m.messages) != 2 || m.messages[0].role != "system" {
		t.Fatalf("messages = %+v, want the banner followed by the user message", m.messages)
	}
	flat := stripANSI(m.msgCache)
	if !strings.Contains(flat, "Type a message to start chatting") {
		t.Errorf("welcome banner missing from transcript:\n%s", flat)
	}
	if !strings.Contains(flat, "┃ You:") || !strings.Contains(flat, "你好") {
		t.Errorf("user message missing from transcript:\n%s", flat)
	}
}

// TestAppendUserMessageKeepsWarnings guards that @mention warnings land in the
// transcript right after the user message.
func TestAppendUserMessageKeepsWarnings(t *testing.T) {
	m := newTestModel(t)
	m.appendUserMessage("见 @missing.bin", []string{"missing.bin: file not found"})

	if len(m.messages) != 3 {
		t.Fatalf("messages = %d, want banner + user + warning", len(m.messages))
	}
	if got := m.messages[2]; got.role != "system" || !strings.Contains(got.content, "file not found") {
		t.Errorf("warning message = %+v", got)
	}
}

// TestBodyWidthUnset guards the pre-resize case: without a terminal width there
// is nothing to wrap against, so wrapping must be a no-op.
func TestBodyWidthUnset(t *testing.T) {
	m := newTestModel(t)
	m.width = 0
	if got := m.bodyWidth(); got != 0 {
		t.Errorf("bodyWidth = %d, want 0", got)
	}
	if got := wrapANSI("abc", m.bodyWidth()); got != "abc" {
		t.Errorf("wrapANSI = %q, want it unchanged", got)
	}
}

// TestWrapANSIWrapsCJK guards that long runs without spaces (Chinese text,
// URLs) are broken instead of being truncated by the viewport.
func TestWrapANSIWrapsCJK(t *testing.T) {
	got := wrapANSI(strings.Repeat("很长的中文说明", 12), 20)
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("long CJK text was not wrapped: %q", got)
	}
	for _, line := range lines {
		if w := lipgloss.Width(line); w > 20 {
			t.Errorf("line width = %d, want <= 20: %q", w, line)
		}
	}
}

// TestWrapANSIKeepsColour guards that the active style is re-applied after an
// inserted break, so wrapped text does not turn uncoloured.
func TestWrapANSIKeepsColour(t *testing.T) {
	got := wrapANSI("\x1b[31m"+strings.Repeat("红色中文", 10)+"\x1b[0m", 10)
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("styled text was not wrapped: %q", got)
	}
	for i, line := range lines[1:] {
		if !strings.HasPrefix(line, "\x1b[31m") {
			t.Errorf("line %d lost its colour: %q", i, line)
		}
	}
}

// TestRenderMessageBlockWrapsCJKMarkdown guards the end-to-end width: a long
// Chinese markdown paragraph must fit the message area, including the bar.
func TestRenderMessageBlockWrapsCJKMarkdown(t *testing.T) {
	m := newTestModel(t)
	m.width = 40
	body := "**说明：**" + strings.Repeat("这是一段很长的中文说明。", 6)

	flat := stripANSI(m.renderOneMessage(message{role: "assistant", content: body}))
	for _, line := range strings.Split(flat, "\n") {
		if w := lipgloss.Width(strings.TrimRight(line, " ")); w > m.bodyWidth()+2 {
			t.Errorf("line width = %d, want <= %d: %q", w, m.bodyWidth()+2, line)
		}
	}
}

// TestInputBoxHeightFollowsDraft guards that the visible input box starts at
// one row, grows with the draft, and never exceeds maxInputLines.
func TestInputBoxHeightFollowsDraft(t *testing.T) {
	m := newTestModel(t)
	if got := m.inputRows; got != 1 {
		t.Fatalf("empty input rows = %d, want 1", got)
	}
	if got := m.viewport.Height; got != 24-1-3 {
		t.Errorf("viewport height = %d, want %d", got, 24-1-3)
	}

	m.input.SetValue("one\ntwo\nthree")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := m.inputRows; got != 3 {
		t.Errorf("3-line draft rows = %d, want 3", got)
	}
	if got := m.viewport.Height; got != 24-3-3 {
		t.Errorf("viewport height = %d, want %d", got, 24-3-3)
	}
	if _, rows := m.inputBlock(); rows != 3 {
		t.Errorf("rendered input rows = %d, want 3", rows)
	}

	m.input.SetValue(strings.Repeat("line\n", 9) + "line")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := m.inputRows; got != maxInputLines {
		t.Errorf("long draft rows = %d, want %d", got, maxInputLines)
	}

	m.input.Reset()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := m.inputRows; got != 1 {
		t.Errorf("rows after reset = %d, want 1", got)
	}
}

// TestViewFillsTerminalHeight guards that the chrome (header + input + status)
// and the viewport always add up to the terminal height, with no blank rows.
func TestViewFillsTerminalHeight(t *testing.T) {
	m := newTestModel(t)
	m.messages = append(m.messages, message{role: "user", content: "hi"})
	m.refreshViewport()

	if got := strings.Count(m.View(), "\n") + 1; got != m.height {
		t.Errorf("view has %d lines, want %d", got, m.height)
	}
}

// TestViewFitsWithConfirmBox guards that the taller tool-confirmation box also
// fits: the viewport gives up the rows it needs.
func TestViewFitsWithConfirmBox(t *testing.T) {
	m := newTestModel(t)
	m.pending = &pendingConfirm{name: "bash", detail: "one\ntwo\nthree\nfour"}
	m.syncLayout()

	if got := strings.Count(m.View(), "\n") + 1; got != m.height {
		t.Errorf("view has %d lines, want %d:\n%s", got, m.height, stripANSI(m.View()))
	}
}

// TestSeparatorAboveInputBox guards the visual anchor above the input: a rule
// spanning the terminal width sits directly above the box, then the status bar.
func TestSeparatorAboveInputBox(t *testing.T) {
	m := newTestModel(t)
	m.messages = append(m.messages, message{role: "user", content: "hi"})
	m.refreshViewport()

	lines := strings.Split(stripANSI(m.View()), "\n")
	if len(lines) != m.height {
		t.Fatalf("view has %d lines, want %d", len(lines), m.height)
	}
	sep := lines[len(lines)-3]
	if sep == "" || strings.Trim(sep, "─") != "" {
		t.Errorf("separator line = %q, want only ─ characters", sep)
	}
	if got := lipgloss.Width(sep); got != m.width-2 {
		t.Errorf("separator width = %d, want %d", got, m.width-2)
	}
	if input := lines[len(lines)-2]; !strings.HasPrefix(input, "┃") {
		t.Errorf("line below the separator = %q, want the input box", input)
	}
}

// TestViewFitsNarrowTerminal guards the same invariant on a narrow terminal
// with a draft: a long model name must not wrap the header into two rows.
func TestViewFillsNarrowTerminal(t *testing.T) {
	m := newTestModel(t)
	m.ready = false
	m.model = "a-very-long-model-name-that-does-not-fit"
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	m.input.SetValue("第一行\n第二行")

	if got := strings.Count(m.View(), "\n") + 1; got != 16 {
		t.Errorf("view has %d lines, want 16:\n%s", got, stripANSI(m.View()))
	}
}

// TestRenderMarkdownWrapsToMessageWidth guards that markdown is wrapped to the
// message area instead of glamour's fixed 80-column default.
func TestRenderMarkdownWrapsToMessageWidth(t *testing.T) {
	m := newTestModel(t)
	m.width = 50
	body := "**Summary:** " + strings.Repeat("this sentence is long enough to need wrapping. ", 5)

	for _, line := range strings.Split(m.renderMarkdown(body), "\n") {
		if w := lipgloss.Width(line); w > m.width-4 {
			t.Errorf("rendered line width = %d, want <= %d: %q", w, m.width-4, stripANSI(line))
		}
	}
}
