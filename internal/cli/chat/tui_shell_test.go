package chat

import (
	"strings"
	"testing"
)

func TestShellDoneUpdatesTranscriptAndHistory(t *testing.T) {
	m := newTestModel(t)
	m.turnID = 7
	m.state = tuiToolCall
	m.messages = append(m.messages, message{
		role:    "tool",
		tool:    "shell",
		content: "Running: echo hi …",
	})

	updated, _ := m.Update(shellDone{turn: 7, cmdLine: "echo hi", result: "hi\n"})
	m = updated.(*chatModel)

	if m.state != tuiIdle {
		t.Errorf("state = %v, want idle", m.state)
	}
	var tool string
	for _, msg := range m.messages {
		if msg.role == "tool" && msg.tool == "shell" {
			tool = msg.content
		}
	}
	if !strings.Contains(tool, "$ echo hi") || !strings.Contains(tool, "hi") {
		t.Errorf("tool message = %q, want the command header and its output", tool)
	}
	if len(m.history) != 1 {
		t.Fatalf("history length = %d, want 1", len(m.history))
	}
	if got := m.history[0].Content; !strings.Contains(got, "echo hi") || !strings.Contains(got, "hi") {
		t.Errorf("history content = %q, want the recorded command and result", got)
	}
}

func TestShellDoneDropsInterruptedResult(t *testing.T) {
	m := newTestModel(t)
	m.turnID = 8
	m.state = tuiToolCall
	m.messages = append(m.messages, message{role: "tool", tool: "shell", content: "Cancelled."})

	updated, _ := m.Update(shellDone{turn: 7, cmdLine: "echo stale", result: "stale"})
	m = updated.(*chatModel)

	if m.state != tuiToolCall {
		t.Errorf("state = %v, want it unchanged for a stale result", m.state)
	}
	if len(m.history) != 0 {
		t.Errorf("history = %+v, want no history for a stale result", m.history)
	}
}
