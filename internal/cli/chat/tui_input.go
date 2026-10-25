package chat

import (
	"context"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

// cancelTurn stops the in-flight turn and invalidates any messages it may still
// send back. Partial streamed output is kept (and marked) instead of being
// silently thrown away, so an interrupted generation is not lost.
func (m *chatModel) cancelTurn() (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	// Bump the turn id first: the cancellation may already have released a
	// chunk or agentDone, and those must not touch the next turn's state.
	m.turnID++

	partial := strings.TrimRight(m.streamBuf.String(), " \t\r\n")
	m.streamBuf.Reset()
	if partial != "" {
		m.messages = append(m.messages, message{
			role:    "assistant",
			content: partial + "\n\n_[interrupted]_",
		})
	}

	// A tool call that was still running when the user interrupted is shown as
	// cancelled rather than forever "Running …".
	for i, tm := range slices.Backward(m.messages) {
		if tm.role == "tool" && strings.Contains(tm.content, "Running ") {
			m.messages[i].content = "Cancelled."
			break
		}
	}

	if m.compacting {
		m.messages = append(m.messages, message{role: "system", content: "Compaction cancelled."})
	}

	m.compacting = false
	m.busy = false
	m.state = tuiIdle
	m.err = nil
	m.input.Focus()
	m.refreshViewport()
	return m, nil
}

// toggleLastToolExpanded expands or collapses the most recent tool output.
func (m *chatModel) toggleLastToolExpanded() (tea.Model, tea.Cmd) {
	for i, msg := range slices.Backward(m.messages) {
		if msg.role == "tool" && msg.tool != "" {
			m.messages[i].expanded = !msg.expanded
			m.refreshViewport()
			break
		}
	}
	return m, nil
}

func (m *chatModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys work regardless of state
	switch msg.Type {
	case tea.KeyCtrlC:
		// While a turn is running Ctrl+C interrupts it; idle Ctrl+C quits.
		// This matches the mental model of claude/codex and avoids killing the
		// whole session when the user only wants to stop a long generation.
		if m.state != tuiIdle {
			return m.cancelTurn()
		}
		m.cancel()
		return m, tea.Quit

	case tea.KeyCtrlD:
		// Don't discard a non-empty draft on an accidental Ctrl+D.
		if strings.TrimSpace(m.input.Value()) != "" {
			return m, nil
		}
		m.cancel()
		return m, tea.Quit

	case tea.KeyEscape:
		if m.state != tuiIdle {
			// Cancel the running operation for real (HTTP request + tools).
			return m.cancelTurn()
		}
		// In idle state, some terminals send ESC separately before Enter
		// as Alt+Enter. We mark altPending so the next Enter is treated
		// as a newline (Alt+Enter) instead of submit.
		// If no Enter follows, the flag is cleared on any other key.
		m.altPending = true
		return m, nil

	case tea.KeyUp:
		return m.handleHistoryPrev()

	case tea.KeyDown:
		return m.handleHistoryNext()

	case tea.KeyTab:
		return m.handleTabCompletion()

	case tea.KeyCtrlO:
		return m.toggleLastToolExpanded()

	case tea.KeyPgUp, tea.KeyPgDown:
		// Page scrolling works even while processing
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.KeyF1:
		m.showHelp()
		return m, nil
	}

	// While processing, ignore most input
	if m.state != tuiIdle {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEnter:
		// Alt+Enter or altPending (ESC then Enter) → insert newline
		if msg.Alt || m.altPending {
			m.altPending = false
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.cycleMatches = nil
			m.cycleIdx = -1
			return m, cmd
		}
		if m.busy || m.compacting {
			return m, nil
		}
		return m.handleSubmitInput()

	case tea.KeyCtrlJ:
		// Ctrl+J (= Ctrl+Enter, 0x0A line feed) → insert newline.
		// Universally reliable on ALL terminals.
		m.altPending = false
		m.cycleMatches = nil
		m.cycleIdx = -1
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return m, cmd

	default:
		// Reset completion cycle on any regular key press
		m.cycleMatches = nil
		m.cycleIdx = -1
		// Any key press cancels a pending Alt sequence
		m.altPending = false
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

// ---------------------------------------------------------------------------
// Command handlers
// ---------------------------------------------------------------------------

// handleUserMessage sends a user message and starts the agent loop.
// handleSubmitInput processes Enter key submission — checks for commands
// or sends the input as a user message to the agent loop.
func (m *chatModel) handleSubmitInput() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.input.Value())
	if input == "" {
		return m, nil
	}

	// Save to history (non-empty, dedup last)
	m.pushHistory(input)

	switch {
	case input == "/exit" || input == "/quit" || input == "/q":
		return m, tea.Quit

	case input == "/clear" || input == "/reset" || input == "/new":
		return m, send(clearChat{})

	case input == "/help" || input == "/?" || input == "?":
		m.showHelp()
		m.input.Reset()
		return m, nil

	case input == "/tools":
		m.showTools("")
		m.input.Reset()
		return m, nil

	case strings.HasPrefix(input, "/tools "):
		m.showTools(strings.TrimSpace(strings.TrimPrefix(input, "/tools")))
		m.input.Reset()
		return m, nil

	case input == "/copy":
		m.handleCopy()
		m.input.Reset()
		return m, nil

	case input == "/compact":
		m.input.Reset()
		m.input.Blur()
		m.state = tuiProcessing
		m.compacting = true
		m.err = nil
		m.turnID++
		turn := m.turnID
		ctx, cancel := context.WithCancel(context.Background())
		m.ctx, m.cancel = ctx, cancel
		m.mu.Lock()
		count := len(m.history)
		m.mu.Unlock()
		go m.compactConversation(ctx, turn, count)
		return m, m.spinner.Tick

	case strings.HasPrefix(input, "/preview"):
		return m.handlePreview(input)

	case strings.HasPrefix(input, "/"):
		return m.handleDirectToolCall(input)

	case strings.HasPrefix(input, "!"):
		return m.handleShellCommand(input)

	default:
		return m.handleUserMessage(input)
	}
}

func (m *chatModel) pushHistory(cmd string) {
	if cmd == "" {
		return
	}
	if len(m.cmdHistory) == 0 || m.cmdHistory[len(m.cmdHistory)-1] != cmd {
		m.cmdHistory = append(m.cmdHistory, cmd)
	}
	m.histIdx = -1
}

// handleHistoryPrev loads the previous history entry (Up arrow).
func (m *chatModel) handleHistoryPrev() (tea.Model, tea.Cmd) {
	if m.state != tuiIdle || len(m.cmdHistory) == 0 {
		return m, nil
	}
	if m.histIdx < len(m.cmdHistory)-1 {
		m.histIdx++
		m.input.SetValue(m.cmdHistory[len(m.cmdHistory)-1-m.histIdx])
		m.input.CursorEnd()
	}
	return m, nil
}

// handleHistoryNext loads the next history entry (Down arrow).
func (m *chatModel) handleHistoryNext() (tea.Model, tea.Cmd) {
	if m.state != tuiIdle {
		return m, nil
	}
	if m.histIdx > 0 {
		m.histIdx--
		m.input.SetValue(m.cmdHistory[len(m.cmdHistory)-1-m.histIdx])
		m.input.CursorEnd()
	} else if m.histIdx == 0 {
		m.histIdx = -1
		m.input.Reset()
	}
	return m, nil
}
