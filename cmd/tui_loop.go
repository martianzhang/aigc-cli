package cmd

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/types"
	"github.com/spf13/cobra"
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

// runChatTUI initialises and runs the Bubble Tea TUI chat program.
func runChatTUI(cmd *cobra.Command) error {
	// Load chat config for agent loop settings
	var chatCfg *types.ChatDefaults
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Chat != nil {
		chatCfg = shared.Cfg.Defaults.Chat
		if shared.Model == "" && chatCfg.Model != "" {
			shared.Model = chatCfg.Model
		}
	}

	maxIterations := 10
	if chatCfg != nil && chatCfg.MaxIterations > 0 {
		maxIterations = chatCfg.MaxIterations
	}

	agentTools := buildAgentTools(chatCfg)
	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)

	// Initialize history with system prompt + current date context
	history := []types.ChatMessage{}
	sysContent := fmt.Sprintf("今天是 %s。你只需要在用户明确询问日期时才回答日期，其他时候不要主动提及。", time.Now().Format("2006年1月2日"))
	if chatSystem != "" {
		sysContent += "\n" + chatSystem
	}
	history = append(history, types.ChatMessage{Role: "system", Content: sysContent})

	model := shared.Model
	if model == "" && chatCfg != nil {
		model = chatCfg.Model
	}

	// Build the TUI model
	tuiModel := newChatModel(c, agentTools, maxIterations, model, chatSystem, cmd, shared.Verbose,
		chatTemperature, chatMaxTokens)
	tuiModel.history = history

	// Create the Bubble Tea program with alt screen
	// (No mouse capture — lets native text selection work)
	p := tea.NewProgram(
		&tuiModel,
		tea.WithAltScreen(),
	)

	// Listen for SIGTERM to cleanly exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		<-sigCh
		p.Quit()
	}()

	// Store program reference for goroutines
	setProgram(p)

	// Run
	_, err := p.Run()
	setProgram(nil)
	return err
}
