package chat

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

// TestMouseWheelScrollsViewport guards the fix for the mouse wheel scrolling the
// terminal scrollback (showing the shell history) instead of the chat viewport.
func TestMouseWheelScrollsViewport(t *testing.T) {
	m := newChatModel(nil, nil, 10, "test-model", "", nil, false, 0, 0, 0)

	// Initialise the viewport through the normal window-size path.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = updated.(*chatModel)

	// Make the content taller than the viewport and start at the bottom.
	m.viewport.SetContent(strings.Repeat("line\n", 200))
	m.viewport.GotoBottom()
	bottom := m.viewport.YOffset
	if bottom == 0 {
		t.Fatal("viewport did not move to the bottom; test setup is invalid")
	}

	updated, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	m = updated.(*chatModel)
	if m.viewport.YOffset >= bottom {
		t.Errorf("wheel up did not scroll the viewport: before=%d after=%d", bottom, m.viewport.YOffset)
	}

	// Scrolling back down returns towards the bottom.
	beforeDown := m.viewport.YOffset
	updated, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	m = updated.(*chatModel)
	if m.viewport.YOffset <= beforeDown {
		t.Errorf("wheel down did not scroll the viewport: before=%d after=%d", beforeDown, m.viewport.YOffset)
	}
}

// TestStreamChunkPreservesScrollPosition ensures streaming output does not yank
// the viewport back to the bottom while the user is reading scrolled-up history.
func TestStreamChunkPreservesScrollPosition(t *testing.T) {
	m := newChatModel(nil, nil, 10, "test-model", "", nil, false, 0, 0, 0)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = updated.(*chatModel)

	m.messages = append(m.messages, message{role: "assistant", content: strings.Repeat("line\n", 200)})
	m.refreshViewport()
	m.viewport.GotoBottom()
	updated, _ = m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	m = updated.(*chatModel)
	scrolled := m.viewport.YOffset
	if scrolled >= m.viewport.TotalLineCount() {
		t.Fatalf("expected to be scrolled up, YOffset=%d", scrolled)
	}

	// A streaming chunk must not move the viewport back to the bottom.
	updated, _ = m.Update(streamChunk{turn: m.turnID, text: "a new streamed token"})
	m = updated.(*chatModel)
	if m.viewport.YOffset != scrolled {
		t.Errorf("streaming changed scroll position: before=%d after=%d", scrolled, m.viewport.YOffset)
	}
}
