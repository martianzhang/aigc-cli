package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

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

				// Take snapshot of history (same principle as runTUIAgentLoop)
				m.mu.Lock()
				localHistory := make([]types.ChatMessage, len(m.history))
				copy(localHistory, m.history)
				m.mu.Unlock()

				// Synthesize an assistant tool_call message so the model
				// sees the full conversational context (assistant requested
				// a tool → tool returned result).
				localHistory = append(localHistory, types.ChatMessage{
					Role:      "assistant",
					Content:   "",
					ToolCalls: []types.ToolCall{tc},
				})

				// Redirect stderr during tool execution
				oldStderr := chatStderr
				chatStderr = &logWriter{prog: prog}
				result := executeToolCall(m.client, tc)
				chatStderr = oldStderr
				summary := summarizeToolResult(cmdName, result)
				prog.Send(toolDone{name: cmdName, summary: summary, content: result})

				localHistory = append(localHistory, types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    result,
				})

				prog.Send(agentDone{history: localHistory})
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

	// Store in history as user message (not system) so it doesn't dilute
	// the system prompt. The model sees it as contextual information
	// provided by the user, not as an instruction-level directive.
	m.history = append(m.history, types.ChatMessage{
		Role:    "user",
		Content: fmt.Sprintf("(I ran the shell command `%s` and got:\n%s)", cmdLine, result),
	})

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
