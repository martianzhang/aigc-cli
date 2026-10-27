package chat

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func (m *chatModel) runTUIAgentLoop(ctx context.Context, turn uint64) {
	prog := getProgram()
	turnCount := 0
	agentStart := time.Now()
	turnClient := m.client.WithContext(ctx)

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
		case <-ctx.Done():
			prog.Send(agentDone{turn: turn, err: ctx.Err(), history: localHistory})
			return
		default:
		}
		turnCount++

		req := &types.ChatRequest{
			Model:      m.model,
			Messages:   localHistory,
			Stream:     true,
			InjectJSON: chatInjectRaw,
		}
		if tools := m.rt.RequestTools(); len(tools) > 0 {
			req.Tools = tools
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
		pw := &progWriter{prog: prog, turn: turn}
		req.OutputWriter = pw

		// Redirect options.Stderr() so tool-execution config output goes to
		// the TUI as logMsg rather than raw stderr.
		restoreStderr := options.SetStderr(&logWriter{prog: prog, turn: turn})
		result, err := turnClient.ChatCompletion(req)
		restoreStderr()

		if err != nil {
			prog.Send(agentDone{turn: turn, err: err, history: localHistory})
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
				select {
				case <-ctx.Done():
					prog.Send(agentDone{turn: turn, err: ctx.Err(), history: localHistory})
					return
				default:
				}

				summary := toolCallSummary(tc.Function.Name, tc.Function.Arguments)
				diff := toolCallDiff(tc.Function.Name, tc.Function.Arguments)
				prog.Send(toolStart{turn: turn, name: tc.Function.Name, summary: summary, diff: diff})

				denied := false
				var answer confirmReply
				if m.confirm.needs(tc.Function.Name, tc.Function.Arguments) {
					detail := confirmDetail(tc.Function.Name, tc.Function.Arguments)
					answer = m.requestToolConfirmation(ctx, prog, turn, tc.Function.Name, summary, detail)
					denied = answer.decision != confirmOnce && answer.decision != confirmAlways
					if ctx.Err() != nil {
						// The user interrupted while the prompt was open.
						prog.Send(agentDone{turn: turn, err: ctx.Err(), history: localHistory})
						return
					}
				}

				var toolResult string
				if denied {
					// Covers both a plain denial and a "do it differently" suggestion:
					// either way the tool does not run, but a suggestion also hands the
					// user's instruction to the model so it can retry in this turn.
					toolResult = deniedToolResult(answer)
					prog.Send(toolDone{
						turn: turn, name: tc.Function.Name,
						summary: deniedSummaryFor(answer), content: toolResult,
					})
				} else {
					// Redirect stderr during tool execution
					restoreStderr := options.SetStderr(&logWriter{prog: prog, turn: turn})
					toolResult = m.rt.Execute(ctx, turnClient, tc)
					restoreStderr()

					prog.Send(toolDone{
						turn: turn, name: tc.Function.Name,
						summary: summarizeToolResult(tc.Function.Name, toolResult), content: toolResult,
					})
				}

				localHistory = append(localHistory, types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    toolResult,
				})
			}

			// Auto-compact if context is getting full. Its progress goes through
			// the same log writer so nothing is written straight to the terminal
			// (which would corrupt the alt-screen).
			if m.contextSize > 0 {
				localHistory = agent.AutoCompactIfNeeded(
					turnClient, m.model, m.temperature, m.maxTokens, m.contextSize,
					localHistory, &logWriter{prog: prog, turn: turn})
			}
			continue
		}

		// Text response — already streamed via progWriter.
		if turnCount >= m.maxIters {
			prog.Send(toolDone{
				turn:    turn,
				name:    "info",
				summary: fmt.Sprintf("Reached maximum iterations (%d). Start a new message to continue.", m.maxIters),
			})
		}

		localHistory = append(localHistory, choice.Message)
		prog.Send(agentDone{
			turn:         turn,
			result:       result,
			elapsed:      time.Since(agentStart),
			err:          nil,
			assistantMsg: &choice.Message,
			history:      localHistory,
		})
		return
	}

	prog.Send(agentDone{turn: turn, history: localHistory})
}

// ---------------------------------------------------------------------------
// Writers
// ---------------------------------------------------------------------------

// progWriter — io.Writer that sends streamChunk messages to a tea.Program.
// Used for AI response streaming content.
type progWriter struct {
	prog *tea.Program
	turn uint64
}

func (w *progWriter) Write(p []byte) (int, error) {
	if w.prog != nil {
		w.prog.Send(streamChunk{turn: w.turn, text: string(p)})
	}
	return len(p), nil
}

var _ io.Writer = (*progWriter)(nil)

// logWriter — io.Writer that sends logMsg messages to a tea.Program.
// Used for capturing options.Stderr() output during tool execution.
type logWriter struct {
	prog *tea.Program
	turn uint64
}

func (w *logWriter) Write(p []byte) (int, error) {
	if w.prog != nil {
		w.prog.Send(logMsg{turn: w.turn, text: string(p)})
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

func getProgram() *tea.Program {
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
