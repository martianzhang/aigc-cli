package cmd

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func (m *chatModel) runTUIAgentLoop() {
	prog := getProgram(m)
	turnCount := 0
	agentStart := time.Now()

	// Take a snapshot of current history (already includes the latest user
	// message appended by handleUserMessage on the main thread).
	// The goroutine works exclusively on this local copy — never on m.history
	// directly — and sends it back via agentDone.history. This avoids the
	// Bubble Tea value-copy trap where a goroutine's mutations to the current
	// Update copy are silently discarded.
	m.mu.Lock()
	localHistory := make([]types.ChatMessage, len(m.history))
	copy(localHistory, m.history)
	m.mu.Unlock()

	for turnCount < m.maxIters {
		select {
		case <-m.ctx.Done():
			prog.Send(agentDone{err: m.ctx.Err(), history: localHistory})
			return
		default:
		}
		turnCount++

		req := &types.ChatRequest{
			Model:    m.model,
			Messages: localHistory,
			Stream:   true,
		}
		if len(m.agentTools) > 0 {
			req.Tools = m.agentTools
		}
		if m.temperature > 0 {
			t := m.temperature
			req.Temperature = &t
		}
		if m.maxTokens > 0 {
			t := m.maxTokens
			req.MaxTokens = &t
		}

		// progWriter sends content to the assistant message area.
		pw := &progWriter{prog: prog}
		req.OutputWriter = pw

		// Redirect chatStderr so tool-execution config output goes to
		// the TUI as logMsg rather than raw stderr.
		oldStderr := chatStderr
		chatStderr = &logWriter{prog: prog}
		result, err := m.client.ChatCompletion(req)
		chatStderr = oldStderr

		if err != nil {
			prog.Send(agentDone{err: err, history: localHistory})
			return
		}
		if len(result.Choices) == 0 {
			break
		}
		choice := result.Choices[0]

		// Tool calls
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			localHistory = append(localHistory, choice.Message)

			for _, tc := range choice.Message.ToolCalls {
				prog.Send(toolStart{name: tc.Function.Name})

				// Redirect stderr during tool execution
				oldStderr := chatStderr
				chatStderr = &logWriter{prog: prog}
				toolResult := executeToolCall(m.client, tc)
				chatStderr = oldStderr

				summary := summarizeToolResult(tc.Function.Name, toolResult)
				prog.Send(toolDone{name: tc.Function.Name, summary: summary, content: toolResult})

				localHistory = append(localHistory, types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    toolResult,
				})
			}

			// Auto-compact if context is getting full
			if m.contextSize > 0 {
				before := estimateHistoryTokens(localHistory)
				localHistory = autoCompactIfNeeded(m.client, m.model, m.temperature, m.maxTokens, m.contextSize, localHistory)
				after := estimateHistoryTokens(localHistory)
				if after < before {
					prog.Send(logMsg(fmt.Sprintf("[auto-compact] %d → %d tokens (saved %d)\r\n",
						before, after, before-after)))
				}
			}
			continue
		}

		// Text response — already streamed via progWriter.
		if turnCount >= m.maxIters {
			prog.Send(toolDone{
				name:    "info",
				summary: fmt.Sprintf("Reached maximum iterations (%d). Start a new message to continue.", m.maxIters),
			})
		}

		localHistory = append(localHistory, choice.Message)
		prog.Send(agentDone{
			result:       result,
			elapsed:      time.Since(agentStart),
			err:          nil,
			assistantMsg: &choice.Message,
			history:      localHistory,
		})
		return
	}

	prog.Send(agentDone{history: localHistory})
}

// ---------------------------------------------------------------------------
// Writers
// ---------------------------------------------------------------------------

// progWriter — io.Writer that sends streamChunk messages to a tea.Program.
// Used for AI response streaming content.
type progWriter struct {
	prog *tea.Program
}

func (w *progWriter) Write(p []byte) (int, error) {
	if w.prog != nil {
		w.prog.Send(streamChunk(string(p)))
	}
	return len(p), nil
}

var _ io.Writer = (*progWriter)(nil)

// logWriter — io.Writer that sends logMsg messages to a tea.Program.
// Used for capturing chatStderr output during tool execution.
type logWriter struct {
	prog *tea.Program
}

func (w *logWriter) Write(p []byte) (int, error) {
	if w.prog != nil {
		w.prog.Send(logMsg(string(p)))
	}
	return len(p), nil
}

var _ io.Writer = (*logWriter)(nil)

func send(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}

// currentProgram and programMu store the active tea.Program reference for
// goroutines (agent loop, tool execution) that need to send messages back.
var (
	currentProgram *tea.Program
	programMu      sync.Mutex
)

func getProgram(m *chatModel) *tea.Program {
	programMu.Lock()
	defer programMu.Unlock()
	return currentProgram
}

func setProgram(p *tea.Program) {
	programMu.Lock()
	currentProgram = p
	programMu.Unlock()
}

// ---------------------------------------------------------------------------
// Entry point — called from runChat when in interactive mode
// ---------------------------------------------------------------------------
