package chat

import (
	"context"
	"fmt"
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
	// Unblock a tool goroutine waiting for confirmation.
	if m.pending != nil {
		p := m.pending
		m.pending = nil
		select {
		case p.reply <- confirmDeny:
		default:
		}
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

// handleConfirmKey answers a pending tool confirmation. Only y/a/n and the
// interrupt keys are accepted; everything else is ignored so the prompt cannot
// be dismissed by accident.
func (m *chatModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyRunes:
		switch strings.ToLower(string(msg.Runes)) {
		case "y":
			return m.resolveConfirm(confirmOnce)
		case "a":
			return m.resolveConfirm(confirmAlways)
		case "n":
			return m.resolveConfirm(confirmDeny)
		}
	case tea.KeyEscape, tea.KeyCtrlC:
		// Cancel the context before answering so the tool goroutine cannot
		// squeeze in an execution between the deny and the turn cancellation.
		if m.cancel != nil {
			m.cancel()
		}
		m.resolveConfirm(confirmDeny)
		return m.cancelTurn()
	}
	return m, nil
}

// resolveConfirm sends the decision to the waiting tool goroutine and restores
// the normal input state.
func (m *chatModel) resolveConfirm(decision confirmDecision) (tea.Model, tea.Cmd) {
	p := m.pending
	if p == nil {
		return m, nil
	}
	m.pending = nil
	if decision == confirmAlways {
		m.confirm.allowAlways(p.name)
	}
	select {
	case p.reply <- decision:
	default:
	}

	if decision == confirmDeny {
		m.state = tuiProcessing
		m.toolMsg = fmt.Sprintf("✗ %s denied by user", p.name)
	} else {
		m.state = tuiToolCall
		m.toolMsg = fmt.Sprintf("🛠️  Running %s …", p.name)
	}
	m.input.Focus()
	return m, nil
}

func (m *chatModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A stale selection (e.g. the release arrived outside the window) should not
	// survive keyboard input.
	if m.selActive {
		m.clearSelection()
	}

	// A pending tool confirmation owns the keyboard until answered.
	if m.pending != nil {
		return m.handleConfirmKey(msg)
	}

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
		// Move within a multi-line draft first; only step back into the command
		// history from the very first line.
		if !m.cursorOnFirstRow() {
			return m, m.updateInput(msg)
		}
		return m.handleHistoryPrev()

	case tea.KeyDown:
		// Mirror of Up: move within the draft until the last line is reached.
		if !m.cursorOnLastRow() {
			return m, m.updateInput(msg)
		}
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

	// While a turn is running the draft can still be edited; only sending it is
	// deferred until the reply finishes (see the Enter case below).
	switch msg.Type {
	case tea.KeyEnter:
		// Any Enter carrying a modifier — Alt+Enter, or the Esc-then-Enter
		// pair some terminals send for it — inserts a line break instead of
		// submitting. Shift/Ctrl+Enter arrive here as Ctrl+J after
		// modifiedEnterReader rewrites them.
		if msg.Alt || m.altPending {
			return m, m.insertNewline()
		}
		if m.busy || m.compacting {
			// A turn is still running: the draft can be edited, but sending it
			// has to wait. Enter just adds a line break so the keystroke is not
			// silently swallowed.
			if strings.TrimSpace(m.input.Value()) == "" {
				return m, nil
			}
			return m, m.insertNewline()
		}
		return m.handleSubmitInput()

	case tea.KeyCtrlJ:
		// Ctrl+J (= Ctrl+Enter, 0x0A line feed) → insert newline.
		// Universally reliable on ALL terminals.
		return m, m.insertNewline()

	default:
		// Reset completion cycle on any regular key press
		m.cycleMatches = nil
		m.cycleIdx = -1
		// Any key press cancels a pending Alt sequence
		m.altPending = false
		return m, m.updateInput(msg)
	}
}

// cursorOnFirstRow reports whether the cursor is on the first visual row of the
// draft (first logical line, first wrapped row).
func (m *chatModel) cursorOnFirstRow() bool {
	li := m.input.LineInfo()
	return m.input.Line() == 0 && li.RowOffset == 0
}

// cursorOnLastRow reports whether the cursor is on the last visual row of the
// draft (last logical line, last wrapped row).
func (m *chatModel) cursorOnLastRow() bool {
	li := m.input.LineInfo()
	return m.input.Line() == m.input.LineCount()-1 && li.RowOffset >= li.Height-1
}

// updateInput forwards a message to the textarea.
func (m *chatModel) updateInput(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

// insertNewline appends a line break to the draft. Modified Enter messages are
// normalised to a plain Enter for the textarea, whose InsertNewline binding
// does not match "alt+enter".
func (m *chatModel) insertNewline() tea.Cmd {
	m.altPending = false
	m.cycleMatches = nil
	m.cycleIdx = -1
	return m.updateInput(tea.KeyMsg{Type: tea.KeyEnter})
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
