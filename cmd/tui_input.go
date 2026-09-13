package cmd

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbletea"
)

func (m *chatModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys work regardless of state
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyCtrlD:
		m.cancel()
		return m, tea.Quit

	case tea.KeyEscape:
		if m.state != tuiIdle {
			// Cancel running operation
			m.cancel()
			m.ctx, m.cancel = context.WithCancel(context.Background())
			m.state = tuiIdle
			m.streamBuf.Reset()
			m.input.Focus()
			m.input.Reset()
			return m, nil
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
		m.showTools()
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
		m.mu.Lock()
		count := len(m.history)
		m.mu.Unlock()
		go m.compactConversation(count)
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
