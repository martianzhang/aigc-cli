package chat

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/cliutil"
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
	anchor := m.usageAnchor
	lastCompaction := m.lastCompaction
	contextTokens := m.contextTokens
	m.mu.Unlock()
	cooldown := m.compactCooldown

	for turnCount < m.maxIters {
		select {
		case <-ctx.Done():
			prog.Send(agentDone{turn: turn, err: ctx.Err(), history: localHistory, contextTokens: contextTokens, lastCompaction: lastCompaction})
			return
		default:
		}
		turnCount++

		tools := m.rt.RequestTools()
		toolsTokens := agent.EstimateToolDefinitions(tools) + agent.EstimateRequestExtras(chatInjectRaw)

		// Auto-compact before building the request. This also covers a resumed
		// session whose stored history is already close to the context limit.
		if cooldown > 0 {
			cooldown--
		} else if m.compactSettings.Enabled && m.compactSettings.Window > 0 {
			lw := &logWriter{prog: prog, turn: turn}
			opts := compactionOptions(m.compactSettings, anchor, toolsTokens, m.sess, m.model, "threshold", false, lw)
			res, cerr := agent.AutoCompact(ctx, turnClient, localHistory, opts)
			if cerr != nil {
				cooldown = 3
				fmt.Fprintf(lw, "[auto-compact] failed: %v\r\n", cerr)
			} else if res != nil {
				localHistory = res.History
				anchor = nil
				lastCompaction = res
				contextTokens = res.Tokens
				if err := persistCompaction(m.sess, res, m.model); err != nil {
					prog.Send(agentDone{turn: turn, err: err, history: localHistory, contextTokens: contextTokens, lastCompaction: lastCompaction})
					return
				}
			}
		}

		var result *types.ChatResponse
		var err error
		recovered := false
		for attempt := 0; attempt < 2; attempt++ {
			req := &types.ChatRequest{
				Model:      m.model,
				Messages:   localHistory,
				Stream:     true,
				InjectJSON: chatInjectRaw,
			}
			if len(tools) > 0 {
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

			// Redirect cliutil.Stderr() so tool-execution config output goes to
			// the TUI as logMsg rather than raw stderr.
			restoreStderr := cliutil.SetStderr(&logWriter{prog: prog, turn: turn})
			result, err = turnClient.ChatCompletion(req)
			pw.Flush()
			restoreStderr()
			if err == nil {
				break
			}
			if recovered || !agent.IsContextOverflow(err) {
				break
			}
			recovered = true
			lw := &logWriter{prog: prog, turn: turn}
			fmt.Fprintf(lw, "[auto-compact] context overflow; compacting and retrying once\r\n")
			opts := compactionOptions(m.compactSettings, anchor, toolsTokens, m.sess, m.model, "overflow", true, lw)
			res, cerr := agent.AutoCompact(ctx, turnClient, localHistory, opts)
			if cerr != nil || res == nil {
				trimmed, keepLast, dropped, ok := agent.EmergencyTrim(localHistory, m.compactSettings, toolsTokens)
				if !ok {
					break
				}
				before := agent.EstimateContext(localHistory, nil, toolsTokens)
				summary := agent.PreviousSummary(trimmed)
				if summary == "" {
					summary = "Context was trimmed locally after compaction failed."
				}
				emergency := &agent.CompactResult{
					Summary: summary, KeepLast: keepLast, TokensBefore: before,
					Tokens: agent.EstimateContext(trimmed, nil, toolsTokens),
					Reason: "emergency", SummaryModel: m.model,
				}
				if dropped > 0 {
					if perr := persistCompaction(m.sess, emergency, m.model); perr != nil {
						prog.Send(agentDone{turn: turn, err: perr, history: localHistory})
						return
					}
				}
				localHistory = trimmed
				anchor = nil
				lastCompaction = emergency
				contextTokens = emergency.Tokens
				fmt.Fprintf(lw, "[auto-compact] dropped %d oldest messages locally\r\n", dropped)
				continue
			}
			localHistory = res.History
			anchor = nil
			lastCompaction = res
			contextTokens = res.Tokens
			if err := persistCompaction(m.sess, res, m.model); err != nil {
				prog.Send(agentDone{turn: turn, err: err, history: localHistory})
				return
			}
		}

		if err != nil {
			prog.Send(agentDone{turn: turn, err: err, history: localHistory, contextTokens: contextTokens, lastCompaction: lastCompaction})
			return
		}
		if result == nil {
			err = fmt.Errorf("chat API returned no response")
			prog.Send(agentDone{turn: turn, err: err, history: localHistory, contextTokens: contextTokens, lastCompaction: lastCompaction})
			return
		}

		if result.Usage != nil && result.Usage.PromptTokens > 0 {
			anchor = &agent.UsageAnchor{MessageIndex: len(localHistory), PromptTokens: result.Usage.PromptTokens}
		}
		contextTokens = agent.EstimateContext(localHistory, anchor, toolsTokens)

		if len(result.Choices) == 0 {
			break
		}
		choice := result.Choices[0]

		// Tool calls
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			localHistory = append(localHistory, choice.Message)
			if m.sess != nil {
				if err := m.sess.AppendMessage(choice.Message, "", m.toolOutputMax); err != nil {
					prog.Send(agentDone{turn: turn, err: err, history: localHistory})
					return
				}
			}

			for i, tc := range choice.Message.ToolCalls {
				select {
				case <-ctx.Done():
					prog.Send(agentDone{turn: turn, err: ctx.Err(), history: localHistory})
					return
				default:
				}

				summary := toolCallSummary(tc.Function.Name, tc.Function.Arguments)
				detail := confirmDetail(tc.Function.Name, tc.Function.Arguments)
				diff := toolCallDiff(tc.Function.Name, tc.Function.Arguments)
				prog.Send(toolStart{turn: turn, name: tc.Function.Name, summary: summary, detail: detail, diff: diff})

				denied := false
				var answer confirmReply
				if m.needsConfirmation(tc.Function.Name, tc.Function.Arguments) {
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
					restoreStderr := cliutil.SetStderr(&logWriter{prog: prog, turn: turn})
					toolResult = m.rt.Execute(ctx, turnClient, tc)
					restoreStderr()

					prog.Send(toolDone{
						turn: turn, name: tc.Function.Name,
						summary: summarizeToolResult(tc.Function.Name, toolResult), content: toolResult,
					})
				}

				toolMessage := types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    toolResult,
				}
				localHistory = append(localHistory, toolMessage)
				if m.sess != nil {
					if err := m.sess.AppendMessage(toolMessage, tc.Function.Name, m.toolOutputMax); err != nil {
						prog.Send(agentDone{turn: turn, err: err, history: localHistory})
						return
					}
				}

				// A plain denial ends the turn right here: the user said no, so
				// the rest of this batch does not run and the model gets no
				// further round trip. A suggestion keeps the turn alive because
				// it is guidance for an immediate retry.
				if denied && denialStopsTurn(answer) {
					var err error
					localHistory, err = m.appendSkippedToolCalls(localHistory, choice.Message.ToolCalls[i+1:], tc.Function.Name)
					if err != nil {
						prog.Send(agentDone{turn: turn, err: err, history: localHistory})
						return
					}
					prog.Send(toolDone{turn: turn, name: "info", summary: denialStopNotice})
					prog.Send(agentDone{turn: turn, history: localHistory})
					return
				}
			}

			continue
		}

		// Text response — already streamed via progWriter.
		if turnCount >= m.maxIters {
			prog.Send(toolDone{
				turn:    turn,
				name:    "info",
				summary: maxIterationsNotice(m.maxIters),
			})
		}

		localHistory = append(localHistory, choice.Message)
		if m.sess != nil {
			if err := m.sess.AppendMessage(choice.Message, "", m.toolOutputMax); err != nil {
				prog.Send(agentDone{turn: turn, err: err, history: localHistory})
				return
			}
		}
		contextTokens = agent.EstimateContext(localHistory, anchor, toolsTokens)
		prog.Send(agentDone{
			turn:            turn,
			result:          result,
			elapsed:         time.Since(agentStart),
			err:             nil,
			assistantMsg:    &choice.Message,
			history:         localHistory,
			contextTokens:   contextTokens,
			lastCompaction:  lastCompaction,
			compactCooldown: cooldown,
			usageAnchor:     anchor,
		})
		return
	}

	// The loop can also end on the tool-call path: the last allowed round-trip
	// returned tool_calls, the tools ran, and the next iteration was refused by
	// the budget. Without an explicit notice the TUI would silently drop back to
	// Ready with the task half-done.
	if turnCount >= m.maxIters {
		prog.Send(toolDone{
			turn:    turn,
			name:    "info",
			summary: maxIterationsNotice(m.maxIters),
		})
	}
	prog.Send(agentDone{turn: turn, history: localHistory})
}

// appendSkippedToolCalls answers the tool calls that were left in the batch
// after a denial, so every tool_call id in the assistant message has a
// matching tool message and the persisted history stays valid for the next
// request.
func (m *chatModel) appendSkippedToolCalls(history []types.ChatMessage, rest []types.ToolCall, deniedName string) ([]types.ChatMessage, error) {
	for _, tc := range rest {
		msg := types.ChatMessage{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    skippedToolCallResult(deniedName),
		}
		history = append(history, msg)
		if m.sess != nil {
			if err := m.sess.AppendMessage(msg, tc.Function.Name, m.toolOutputMax); err != nil {
				return history, err
			}
		}
	}
	return history, nil
}

// ---------------------------------------------------------------------------
// Writers
// ---------------------------------------------------------------------------

// progWriter — io.Writer that sends streamChunk messages to a tea.Program.
// Deltas are coalesced for a few dozen milliseconds: one tea.Msg per SSE token
// makes the TUI re-render (and copy the transcript) thousands of times.
type progWriter struct {
	prog *tea.Program
	turn uint64

	mu    sync.Mutex
	buf   strings.Builder
	timer *time.Timer
}

const streamFlushInterval = 40 * time.Millisecond

func (w *progWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	if w.timer == nil {
		w.timer = time.AfterFunc(streamFlushInterval, w.flush)
	}
	w.mu.Unlock()
	return len(p), nil
}

// Flush sends any buffered text immediately. Call it when the stream ends so
// the tail is never lost.
func (w *progWriter) Flush() {
	w.mu.Lock()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.mu.Unlock()
	w.flush()
}

func (w *progWriter) flush() {
	w.mu.Lock()
	text := w.buf.String()
	w.buf.Reset()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.mu.Unlock()
	if text != "" && w.prog != nil {
		w.prog.Send(streamChunk{turn: w.turn, text: text})
	}
}

var _ io.Writer = (*progWriter)(nil)

// logWriter — io.Writer that sends logMsg messages to a tea.Program.
// Used for capturing cliutil.Stderr() output during tool execution.
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
