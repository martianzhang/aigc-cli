package chat

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestEveryCommandClearsInput guards the shared contract: a submitted command
// must never remain in the input box, regardless of which branch handles it.
func TestEveryCommandClearsInput(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	commands := []string{
		"/help",
		"/tools",
		"/skills",
		"/name",
		"/name demo",
		"/session",
		"/sessions",
		"/resume",
		"/session 1",
		"/resume missing",
		"/session find",
		"/session find demo",
		"/sessions find demo",
		"/save",
		"/save demo",
		"/context",
		"/model",
		"/model gpt-4o",
		"/model default",
		"/model alt gpt-4o",
		"/provider",
		"/provider alt",
		"/provider default",
		"/preview",
		"/skill:",
		"/skill:missing",
		"/unknown-tool",
		"/clear",
		"!",
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			m := switchTestModel()
			defer closeTestSession(m)
			m.sessionDirPath = t.TempDir()
			m.width, m.height = 100, 30
			m.input.SetValue(command)
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := next.(*chatModel)
			if value := got.input.Value(); value != "" {
				t.Fatalf("%q left input behind: %q", command, value)
			}
			if got.inputRows != 1 {
				t.Fatalf("%q inputRows = %d, want 1", command, got.inputRows)
			}
		})
	}
}
