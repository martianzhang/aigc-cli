package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
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

// pushHistory adds a non-empty command to history, deduplicating the last entry.
// compactConversation sends the full history to the API for summarisation
// and sends a compactDone message back to the TUI with the result.
func (m *chatModel) compactConversation(count int) {
	prog := getProgram(m)

	m.mu.Lock()
	history := make([]types.ChatMessage, len(m.history))
	copy(history, m.history)
	m.mu.Unlock()

	if len(history) == 0 {
		prog.Send(compactDone{err: fmt.Errorf("no conversation to compact")})
		return
	}

	summarizePrompt := `Please provide a detailed summary of our conversation above. Capture: 1) the user's goals and requirements, 2) key decisions made, 3) any files created or modified, 4) current status of any ongoing work. Be thorough — this summary will replace the conversation history so nothing important should be lost.`

	req := &types.ChatRequest{
		Model:    m.model,
		Messages: append(history, types.ChatMessage{Role: "user", Content: summarizePrompt}),
		Stream:   false,
	}
	if m.temperature > 0 {
		t := m.temperature
		req.Temperature = &t
	}
	if m.maxTokens > 0 {
		t := m.maxTokens
		req.MaxTokens = &t
	}

	result, err := m.client.ChatCompletion(req)
	if err != nil {
		prog.Send(compactDone{err: err})
		return
	}

	if len(result.Choices) == 0 {
		prog.Send(compactDone{err: fmt.Errorf("API returned no choices")})
		return
	}

	summary := result.Choices[0].Message.Content

	// Replace history with the original system prompt + compacted summary
	m.mu.Lock()
	if m.system != "" {
		m.history = []types.ChatMessage{
			{Role: "system", Content: m.system},
			{Role: "system", Content: "Previous conversation summary:\n\n" + summary},
		}
	} else {
		m.history = []types.ChatMessage{
			{Role: "system", Content: "Previous conversation summary:\n\n" + summary},
		}
	}
	m.mu.Unlock()

	prog.Send(compactDone{summary: fmt.Sprintf("Compacted %d messages into 1 summary:\n\n%s", count, summary)})
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

func (m *chatModel) handleUserMessage(input string) (tea.Model, tea.Cmd) {
	// Remove initial welcome message on first real message
	if len(m.messages) == 1 && m.messages[0].role == "system" {
		m.messages = nil
	}
	// Add user message to display
	m.messages = append(m.messages, message{role: "user", content: input})

	// Add to API history
	m.mu.Lock()
	m.history = append(m.history, types.ChatMessage{Role: "user", Content: input})
	m.mu.Unlock()
	m.input.Reset()
	m.input.Blur()

	// Switch state
	m.state = tuiProcessing
	m.busy = true
	m.streamBuf.Reset()
	m.err = nil

	// Render user message immediately
	m.refreshViewport()

	// Start agent loop in a goroutine
	go m.runTUIAgentLoop()

	return m, m.spinner.Tick
}

// handlePreview processes the /preview command.
func (m *chatModel) handlePreview(input string) (tea.Model, tea.Cmd) {
	parts := strings.SplitN(input, " ", 2)
	filePath := ""
	if len(parts) == 2 {
		filePath = strings.TrimSpace(parts[1])
	}
	if filePath == "" {
		m.messages = append(m.messages, message{
			role:    "system",
			content: "Usage: /preview <filepath>",
		})
		m.input.Reset()
		rendered := m.renderMessages()
		m.viewport.SetContent(rendered)
		m.viewport.GotoBottom()
		return m, nil
	}

	// Temporarily switch out of alt screen for the preview
	go func() {
		prog := getProgram(m)
		if err := service.PreviewFile(filePath); err != nil {
			prog.Send(logMsg(fmt.Sprintf("Preview failed: %v", err)))
		}
	}()

	m.messages = append(m.messages, message{
		role:    "system",
		content: fmt.Sprintf("Previewing: %s", filePath),
	})
	rendered := m.renderMessages()
	m.viewport.SetContent(rendered)
	m.viewport.GotoBottom()
	return m, nil
}

// handleDirectToolCall processes a /toolname JSON direct call.
func (m *chatModel) handleDirectToolCall(input string) (tea.Model, tea.Cmd) {
	spaceIdx := strings.Index(input, " ")
	cmdName := input[1:]
	argsJSON := ""
	if spaceIdx > 0 {
		cmdName = input[1:spaceIdx]
		argsJSON = strings.TrimSpace(input[spaceIdx+1:])
	}

	// Find matching tool
	for _, t := range m.agentTools {
		if t.Function.Name == cmdName {
			tc := types.ToolCall{
				ID:   "direct",
				Type: "function",
				Function: types.ToolCallFunction{
					Name:      cmdName,
					Arguments: argsJSON,
				},
			}

			m.input.Reset()
			m.input.Blur()
			m.state = tuiToolCall
			m.toolMsg = fmt.Sprintf("🛠️  Direct call: %s …", cmdName)

			// Add pending message
			m.messages = append(m.messages, message{
				role:    "tool",
				content: fmt.Sprintf("Running %s …", cmdName),
				tool:    cmdName,
			})

			// Run in goroutine
			go func() {
				prog := getProgram(m)
				prog.Send(toolStart{name: cmdName})
				// Redirect stderr during tool execution
				oldStderr := chatStderr
				chatStderr = &logWriter{prog: prog}
				result := executeToolCall(m.client, tc)
				chatStderr = oldStderr
				summary := summarizeToolResult(cmdName, result)
				prog.Send(toolDone{name: cmdName, summary: summary, content: result})

				// Also add to conversation history as a tool message
				m.mu.Lock()
				m.history = append(m.history, types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result,
				})
				m.mu.Unlock()

				prog.Send(agentDone{})
			}()

			return m, m.spinner.Tick
		}
	}

	// Unknown command — show as message
	m.messages = append(m.messages, message{
		role:    "system",
		content: fmt.Sprintf("Unknown tool: %s. Type /tools to list available tools.", cmdName),
	})
	m.input.Reset()
	rendered := m.renderMessages()
	m.viewport.SetContent(rendered)
	m.viewport.GotoBottom()
	return m, nil
}

// handleShellCommand executes a !command synchronously.
// Shell commands are usually instant (< 1s), so synchronous execution avoids
// race conditions where the next user message is processed before the
// shell output is written to history.
func (m *chatModel) handleShellCommand(input string) (tea.Model, tea.Cmd) {
	cmdLine := strings.TrimSpace(input[1:])
	if cmdLine == "" {
		return m, nil
	}

	m.input.Reset()
	m.input.Blur()
	m.state = tuiToolCall
	m.toolMsg = fmt.Sprintf("Running: %s", cmdLine)

	m.messages = append(m.messages, message{
		role:    "tool",
		content: fmt.Sprintf("Running: %s \u2026", cmdLine),
		tool:    "shell",
	})
	m.refreshViewport()

	// Run synchronously — shell commands are fast and this avoids
	// race conditions with subsequent user messages.
	result := executeShellCommand(cmdLine)

	// Store in history as system message so the model treats it as
	// contextual information rather than a user query.
	m.mu.Lock()
	m.history = append(m.history, types.ChatMessage{
		Role:    "system",
		Content: fmt.Sprintf("Shell command `%s` returned:\n%s", cmdLine, result),
	})
	m.mu.Unlock()

	// Update the tool message with the result
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].role == "tool" && m.messages[i].tool == "shell" {
			m.messages[i].content = result
			break
		}
	}
	m.state = tuiIdle
	m.input.Focus()
	m.refreshViewport()

	return m, nil
}

// ---------------------------------------------------------------------------
// TUI agent loop — runs in a goroutine, sends tea.Msg to the program
// ---------------------------------------------------------------------------
