package chat

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// newTestModel returns a model with a viewport ready to render.
func newTestModel(t *testing.T) *chatModel {
	t.Helper()
	m := newChatModel(nil, nil, 10, "test-model", "", nil, false, 0, 0, 0)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return updated.(*chatModel)
}

// TestCancelTurnKeepsPartialOutput guards that interrupting a streaming turn
// preserves what was already generated instead of silently discarding it.
func TestCancelTurnKeepsPartialOutput(t *testing.T) {
	m := newTestModel(t)
	m.state = tuiProcessing
	m.busy = true
	m.streamBuf.WriteString("half an answer")
	m.messages = append(m.messages, message{
		role:    "tool",
		content: "Running bash …",
		tool:    "bash",
	})

	m.cancelTurn()

	if m.state != tuiIdle || m.busy {
		t.Fatalf("state = %v busy = %v, want idle/false", m.state, m.busy)
	}
	var assistant, tool string
	for _, msg := range m.messages {
		if msg.role == "assistant" {
			assistant = msg.content
		}
		if msg.role == "tool" {
			tool = msg.content
		}
	}
	if !strings.Contains(assistant, "half an answer") || !strings.Contains(assistant, "[interrupted]") {
		t.Errorf("partial assistant output = %q, want it preserved with an interrupt marker", assistant)
	}
	if tool != "Cancelled." {
		t.Errorf("running tool message = %q, want Cancelled.", tool)
	}
}

// TestCancelTurnDropsLateMessages guards against a cancelled turn's streaming
// chunks and agentDone overwriting the state of the next turn.
func TestCancelTurnDropsLateMessages(t *testing.T) {
	m := newTestModel(t)
	m.turnID = 5
	m.state = tuiProcessing
	m.streamBuf.WriteString("keep me")

	// A chunk from the cancelled turn must not be appended.
	updated, _ := m.Update(streamChunk{turn: 4, text: " late chunk"})
	m = updated.(*chatModel)
	if got := m.streamBuf.String(); got != "keep me" {
		t.Errorf("streamBuf = %q, stale chunk was applied", got)
	}

	// A stale agentDone must not replace history.
	original := []types.ChatMessage{{Role: "user", Content: "current"}}
	m.history = original
	updated, _ = m.Update(agentDone{turn: 4, history: []types.ChatMessage{{Role: "user", Content: "stale"}}})
	m = updated.(*chatModel)
	if len(m.history) != 1 || m.history[0].Content != "current" {
		t.Errorf("history = %+v, stale agentDone was applied", m.history)
	}

	// The current turn still works.
	updated, _ = m.Update(agentDone{turn: 5, history: []types.ChatMessage{{Role: "user", Content: "fresh"}}})
	m = updated.(*chatModel)
	if len(m.history) != 1 || m.history[0].Content != "fresh" {
		t.Errorf("history = %+v, current agentDone was dropped", m.history)
	}
}

// TestCtrlCInterruptsRunningTurn ensures Ctrl+C stops the turn instead of
// quitting the whole session while a generation is in flight.
func TestCtrlCInterruptsRunningTurn(t *testing.T) {
	m := newTestModel(t)
	m.state = tuiProcessing
	m.busy = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(*chatModel)
	if cmd != nil {
		t.Error("Ctrl+C during a turn returned a command; expected a plain interrupt")
	}
	if m.state != tuiIdle || m.busy {
		t.Errorf("state = %v busy = %v, want idle/false", m.state, m.busy)
	}
}

// TestCtrlCIdleQuits keeps the familiar behaviour when nothing is running.
func TestCtrlCIdleQuits(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("Ctrl+C while idle did not return a quit command")
	}
}

// TestCtrlDKeepsDraft guards against losing typed input to an accidental Ctrl+D.
func TestCtrlDKeepsDraft(t *testing.T) {
	m := newTestModel(t)
	m.input.SetValue("draft message")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = updated.(*chatModel)
	if cmd != nil {
		t.Error("Ctrl+D with a non-empty draft returned a quit command")
	}
	if got := m.input.Value(); got != "draft message" {
		t.Errorf("input = %q, draft was lost", got)
	}
}

// TestEscCancelsTurn checks the Esc path mirrors Ctrl+C.
func TestEscCancelsTurn(t *testing.T) {
	m := newTestModel(t)
	m.state = tuiToolCall
	m.busy = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(*chatModel)
	if cmd != nil {
		t.Error("Esc returned a command; expected a plain cancel")
	}
	if m.state != tuiIdle {
		t.Errorf("state = %v, want idle", m.state)
	}
}
