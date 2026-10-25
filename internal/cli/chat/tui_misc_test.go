package chat

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

func TestClearChatRefreshesRenderedCache(t *testing.T) {
	m := newTestModel(t)
	m.system = "sys"
	m.messages = append(m.messages, message{role: "user", content: "old conversation"})
	m.refreshViewport()

	updated, _ := m.Update(clearChat{})
	m = updated.(*chatModel)

	if !strings.Contains(m.msgCache, "Type a message to start chatting") {
		t.Errorf("msgCache does not contain the welcome banner:\n%s", m.msgCache)
	}
	if strings.Contains(m.msgCache, "old conversation") {
		t.Error("msgCache still contains the cleared conversation")
	}
	if m.viewport.View() == "" {
		t.Error("viewport is empty after /clear")
	}
	if len(m.history) != 1 || m.history[0].Content != "sys" {
		t.Errorf("history = %+v, want only the system prompt", m.history)
	}
}

func TestRenderStatusClipsLongError(t *testing.T) {
	m := newTestModel(t)
	m.width = 50
	m.err = errors.New(strings.Repeat("very long error ", 20))

	status := m.renderStatus()
	if w := lipgloss.Width(status); w > m.width-2 {
		t.Errorf("status width = %d, want <= %d: %q", w, m.width-2, status)
	}
	if !strings.Contains(status, "…") {
		t.Errorf("clipped status should end with an ellipsis: %q", status)
	}
}

func TestSummarizeToolResultRuneSafe(t *testing.T) {
	got := summarizeToolResult("x", strings.Repeat("你好", 100))
	if !utf8.ValidString(got) {
		t.Errorf("summarizeToolResult produced invalid UTF-8: %q", got)
	}
	if w := lipgloss.Width(got); w > 80 {
		t.Errorf("summarizeToolResult width = %d, want <= 80", w)
	}
}

func TestShortDescriptionRuneSafe(t *testing.T) {
	got := shortDescription(strings.Repeat("描述", 100))
	if !utf8.ValidString(got) {
		t.Errorf("shortDescription produced invalid UTF-8: %q", got)
	}
	if w := lipgloss.Width(got); w > 100 {
		t.Errorf("shortDescription width = %d, want <= 100", w)
	}
}

func TestNormalizeLogText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"progress keeps final state", "10%\r50%\r100%\n", "100%"},
		{"crlf becomes lf", "a\r\nb\r\n", "a\nb"},
		{"bare cr within a line", "loading\r\ndone", "loading\ndone"},
		{"trailing newlines dropped", "x\n\n\n", "x"},
		{"empty", "\r\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeLogText(tt.in); got != tt.want {
				t.Errorf("normalizeLogText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Guard that clearChat keeps the TUI usable: a streamed chunk after clearing
// must show only the new content, not the old transcript.
func TestStreamAfterClearStartsWithBanner(t *testing.T) {
	m := newTestModel(t)
	m.messages = append(m.messages, message{role: "user", content: "old"})
	m.refreshViewport()

	updated, _ := m.Update(clearChat{})
	m = updated.(*chatModel)
	updated, _ = m.Update(streamChunk{turn: m.turnID, text: "new answer"})
	m = updated.(*chatModel)

	rendered := m.renderMessagesWithStream()
	if strings.Contains(rendered, "old") {
		t.Errorf("rendered stream still contains the cleared transcript:\n%s", rendered)
	}
	if !strings.Contains(rendered, "new answer") {
		t.Errorf("rendered stream is missing the new answer:\n%s", rendered)
	}
}
