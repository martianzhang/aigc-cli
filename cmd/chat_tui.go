// Package cmd — Bubble Tea TUI for interactive chat.
//
// This file implements a Model-View-Update (MVU) chat interface using
// github.com/charmbracelet/bubbletea. It replaces the old raw-terminal
// input loop (runInteractiveChat) with a modern TUI experience.

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// TUI state machine
// ---------------------------------------------------------------------------

type tuiState int

const (
	tuiIdle       tuiState = iota // waiting for user input
	tuiProcessing                 // agent loop running, streaming response
	tuiToolCall                   // tool call in progress
)

// ---------------------------------------------------------------------------
// Tea message types
// ---------------------------------------------------------------------------

// streamChunk is sent for each streaming content delta from the API.
type streamChunk string

// logMsg is sent for status/log output during tool execution (e.g. config info).
type logMsg string

// toolStart is sent when a tool call begins.
type toolStart struct {
	name string
}

// toolDone is sent when a tool call completes.
type toolDone struct {
	name    string // tool name
	summary string // one-line summary for status bar
	content string // full output for the message area (empty = use summary)
}

// agentDone is sent when the agent loop finishes for one user turn.
type agentDone struct {
	result  *types.ChatResponse
	elapsed time.Duration
	err     error
}

// clearChat signals clearing the conversation.
type clearChat struct{}

// ---------------------------------------------------------------------------
// Message model
// ---------------------------------------------------------------------------

// message represents one entry in the TUI message list.
type message struct {
	role    string // "user", "assistant", "system", "tool"
	content string
	tool    string // tool name for tool messages
}

// ---------------------------------------------------------------------------
// Bubble Tea Model
// ---------------------------------------------------------------------------

// chatModel is the top-level Bubble Tea model for the chat TUI.
type chatModel struct {
	// Terminal dimensions
	width  int
	height int
	ready  bool // viewport initialised

	// State machine
	state   tuiState
	err     error
	toolMsg string // current tool call description shown in status

	// Chat history displayed in the viewport
	messages []message

	// Streaming accumulator — content being built for the current assistant
	// turn. Flushed into messages[] when the turn finishes.
	streamBuf *strings.Builder

	// Conversation history for API requests (types.ChatMessage, not TUI message)
	history []types.ChatMessage

	// Dependencies
	client     *client.Client
	agentTools []types.ToolDefinition
	maxIters   int
	model      string
	system     string
	cmd        *cobra.Command
	verbose    bool

	// Cobra flag values used when building requests
	temperature float64
	maxTokens   int

	// UI components
	viewport viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	mu     *sync.Mutex // guards history writes from agent goroutine

	// Tab completion state
	completions  []string // all possible completions (built on init)
	cycleMatches []string // current completion cycle matches
	cycleIdx     int      // index into cycleMatches

	// altPending is set when Escape is pressed in idle state.
	// On some terminals, Alt+Enter sends ESC then Enter separately;
	// altPending allows us to detect this as a newline intent.
	altPending bool

	// Command history for Up/Down arrow navigation
	cmdHistory []string
	histIdx    int // -1 = new input, 0+ = index into cmdHistory

	// Styling
	styles chatStyles
}

type chatStyles struct {
	header        lipgloss.Style
	messages      lipgloss.Style
	inputBox      lipgloss.Style
	shellInputBox lipgloss.Style
	statusBar     lipgloss.Style
	userMsg       lipgloss.Style
	asstMsg       lipgloss.Style
	sysMsg        lipgloss.Style
	toolMsg       lipgloss.Style
	errMsg        lipgloss.Style
}

func defaultChatStyles() chatStyles {
	subtle := lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}
	userCol := lipgloss.AdaptiveColor{Light: "#2277DD", Dark: "#66AAFF"}
	asstCol := lipgloss.AdaptiveColor{Light: "#22AA66", Dark: "#55DD99"}
	toolCol := lipgloss.AdaptiveColor{Light: "#AA6622", Dark: "#DDAA55"}
	errCol := lipgloss.AdaptiveColor{Light: "#DD2222", Dark: "#FF5555"}

	return chatStyles{
		header: lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("#335577")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Width(80),
		messages: lipgloss.NewStyle().
			Padding(0, 1),
		inputBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#335577")).
			Padding(0, 1),
		shellInputBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#DD8833")).
			Padding(0, 1),
		statusBar: lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("#222222")).
			Foreground(lipgloss.Color("#CCCCCC")),
		userMsg: lipgloss.NewStyle().
			Foreground(userCol).
			Bold(true),
		asstMsg: lipgloss.NewStyle().
			Foreground(asstCol),
		sysMsg: lipgloss.NewStyle().
			Foreground(subtle).
			Italic(true),
		toolMsg: lipgloss.NewStyle().
			Foreground(toolCol),
		errMsg: lipgloss.NewStyle().
			Foreground(errCol),
	}
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

func newChatModel(c *client.Client, tools []types.ToolDefinition, maxIt int, mdl, sys string,
	cobraCmd *cobra.Command, verb bool, temp float64, maxTok int) chatModel {

	ctx, cancel := context.WithCancel(context.Background())

	ti := textarea.New()
	ti.Focus()
	ti.Placeholder = "Enter to send, Alt+Enter / Ctrl+J for newline"
	ti.Prompt = ""
	ti.ShowLineNumbers = false
	ti.CharLimit = 0
	ti.MaxWidth = 80
	ti.MaxHeight = 5
	// Minimal style — no cursor line highlight
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()

	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#335577", Dark: "#66AAFF"})
	s.Spinner = spinner.Dot

	// Build completion list from tool definitions
	completions := []string{"/exit", "/quit", "/q", "/clear", "/reset", "/help", "/?", "/tools"}
	for _, t := range tools {
		completions = append(completions, "/"+t.Function.Name)
	}

	return chatModel{
		state:       tuiIdle,
		client:      c,
		agentTools:  tools,
		maxIters:    maxIt,
		model:       mdl,
		system:      sys,
		cmd:         cobraCmd,
		verbose:     verb,
		temperature: temp,
		maxTokens:   maxTok,
		input:       ti,
		spinner:     s,
		completions: completions,
		cycleIdx:    -1,
		histIdx:     -1,
		streamBuf:   &strings.Builder{},
		mu:          &sync.Mutex{},
		ctx:         ctx,
		cancel:      cancel,
		styles:      defaultChatStyles(),
	}
}

// ---------------------------------------------------------------------------
// Init — Bubble Tea lifecycle
// ---------------------------------------------------------------------------

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		textarea.Blink,
	)
}

// ---------------------------------------------------------------------------
// Update — the "brain" of the MVU pattern
// ---------------------------------------------------------------------------

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// ---- window / terminal events ----------------------------------------
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.styles.header = m.styles.header.Width(msg.Width - 2)
		m.input.SetWidth(msg.Width - 6)
		if !m.ready {
			m.viewport = viewport.New(msg.Width-2, msg.Height-8)
			m.viewport.YPosition = 1
			m.viewport.Style = m.styles.messages
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 2
			m.viewport.Height = msg.Height - 8
		}
		m.viewport.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	// ---- streaming -------------------------------------------------------
	case streamChunk:
		m.streamBuf.WriteString(string(msg))
		rendered := m.renderMessagesWithStream()
		m.viewport.SetContent(rendered)
		m.viewport.GotoBottom()
		return m, nil

	// ---- log messages (stderr redirect from tool execution) --------------
	case logMsg:
		text := strings.TrimRight(string(msg), "\r\n")
		if text != "" {
			m.messages = append(m.messages, message{role: "system", content: text})
			m.refreshViewport()
		}
		return m, nil

	// ---- tool calls ------------------------------------------------------
	case toolStart:
		m.state = tuiToolCall
		m.toolMsg = fmt.Sprintf("🛠️  Running %s …", msg.name)
		m.messages = append(m.messages, message{
			role:    "tool",
			content: fmt.Sprintf("Running %s …", msg.name),
			tool:    msg.name,
		})
		m.refreshViewport()
		return m, m.spinner.Tick

	case toolDone:
		m.state = tuiProcessing
		m.toolMsg = fmt.Sprintf("✓ %s: %s", msg.name, msg.summary)
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "tool" && m.messages[i].tool == msg.name {
				content := msg.content
				if content == "" {
					content = fmt.Sprintf("✓ %s", msg.summary)
				}
				m.messages[i].content = content
				break
			}
		}
		m.refreshViewport()
		return m, nil

	// ---- agent loop finished ---------------------------------------------
	case agentDone:
		m.state = tuiIdle
		// Flush stream buffer into a proper message
		if m.streamBuf.Len() > 0 {
			m.messages = append(m.messages, message{
				role:    "assistant",
				content: m.streamBuf.String(),
			})
		}
		m.streamBuf.Reset()
		m.input.Focus()
		m.input.Reset()

		if msg.err != nil {
			if msg.err == context.Canceled {
				m.err = nil
			} else {
				m.err = msg.err
				m.messages = append(m.messages, message{
					role:    "system",
					content: fmt.Sprintf("Error: %v", msg.err),
				})
			}
		} else if msg.result != nil && m.verbose {
			// Verbose stats
			parts := []string{}
			if msg.result.Model != "" {
				parts = append(parts, fmt.Sprintf("Model: %s", msg.result.Model))
			}
			if msg.result.Usage != nil {
				parts = append(parts, fmt.Sprintf("Tokens: %d↑ + %d↓ = %d",
					msg.result.Usage.PromptTokens, msg.result.Usage.CompletionTokens, msg.result.Usage.TotalTokens))
				if msg.result.Usage.Cost > 0 {
					parts = append(parts, fmt.Sprintf("Cost: $%.6f", msg.result.Usage.Cost))
				}
			}
			parts = append(parts, fmt.Sprintf("Time: %v", msg.elapsed.Round(time.Millisecond)))
			if len(parts) > 0 {
				m.messages = append(m.messages, message{
					role:    "system",
					content: "──  " + strings.Join(parts, "  │  "),
				})
			}
		}
		m.refreshViewport()
		return m, nil

	// ---- clear chat ------------------------------------------------------
	case clearChat:
		m.messages = nil
		m.history = nil
		if m.system != "" {
			m.history = append(m.history, types.ChatMessage{Role: "system", Content: m.system})
		}
		m.streamBuf.Reset()
		m.err = nil
		m.input.Reset()
		m.viewport.SetContent("")
		m.viewport.GotoBottom()
		return m, nil

	// ---- spinner tick ----------------------------------------------------
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	// ---- textinput updates (delegated) -----------------------------------
	default:
		if !m.ready {
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

// handleKeyMsg processes keyboard events.
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

	case input == "/clear" || input == "/reset":
		return m, send(clearChat{})

	case input == "/help" || input == "/?" || input == "?":
		m.showHelp()
		m.input.Reset()
		return m, nil

	case input == "/tools":
		m.showTools()
		m.input.Reset()
		return m, nil

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
	// Add user message to display
	m.messages = append(m.messages, message{role: "user", content: input})

	// Add to API history
	m.mu.Lock()
	m.history = append(m.history, types.ChatMessage{Role: "user", Content: input})
	m.mu.Unlock()

	// Clear input
	m.input.Reset()
	m.input.Blur()

	// Switch state
	m.state = tuiProcessing
	m.streamBuf.Reset()
	m.err = nil

	// Render user message immediately
	rendered := m.renderMessages()
	m.viewport.SetContent(rendered)
	m.viewport.GotoBottom()

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

// handleShellCommand executes a !command.
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
		content: fmt.Sprintf("Running: %s …", cmdLine),
		tool:    "shell",
	})

	go func() {
		prog := getProgram(m)
		result := executeShellCommand(cmdLine)
		summary := summarizeToolResult("shell", result)
		// For shell commands, show the full output instead of just the summary
		prog.Send(toolDone{name: "shell", summary: summary, content: result})
		prog.Send(agentDone{})
	}()

	return m, m.spinner.Tick
}

// ---------------------------------------------------------------------------
// TUI agent loop — runs in a goroutine, sends tea.Msg to the program
// ---------------------------------------------------------------------------

func (m *chatModel) runTUIAgentLoop() {
	prog := getProgram(m)
	turnCount := 0
	agentStart := time.Now()

	for turnCount < m.maxIters {
		select {
		case <-m.ctx.Done():
			prog.Send(agentDone{err: m.ctx.Err()})
			return
		default:
		}
		turnCount++

		m.mu.Lock()
		req := &types.ChatRequest{
			Model:    m.model,
			Messages: m.history,
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
		m.mu.Unlock()

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
			prog.Send(agentDone{err: err})
			return
		}
		if len(result.Choices) == 0 {
			break
		}
		choice := result.Choices[0]

		// Tool calls
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			m.mu.Lock()
			m.history = append(m.history, choice.Message)
			m.mu.Unlock()

			for _, tc := range choice.Message.ToolCalls {
				prog.Send(toolStart{name: tc.Function.Name})

				// Redirect stderr during tool execution
				oldStderr := chatStderr
				chatStderr = &logWriter{prog: prog}
				toolResult := executeToolCall(m.client, tc)
				chatStderr = oldStderr

				summary := summarizeToolResult(tc.Function.Name, toolResult)
				prog.Send(toolDone{name: tc.Function.Name, summary: summary, content: toolResult})

				m.mu.Lock()
				m.history = append(m.history, types.ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    toolResult,
				})
				m.mu.Unlock()
			}
			continue
		}

		// Text response — already streamed via progWriter
		m.mu.Lock()
		m.history = append(m.history, choice.Message)
		m.mu.Unlock()

		if turnCount >= m.maxIters {
			prog.Send(toolDone{
				name:    "info",
				summary: fmt.Sprintf("Reached maximum iterations (%d). Start a new message to continue.", m.maxIters),
			})
		}

		prog.Send(agentDone{result: result, elapsed: time.Since(agentStart), err: nil})
		return
	}

	prog.Send(agentDone{})
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

// ---------------------------------------------------------------------------
// View — renders the entire TUI
// ---------------------------------------------------------------------------

func (m chatModel) View() string {
	if !m.ready {
		return "\n  Initializing…"
	}

	var b strings.Builder

	// Header
	headerText := fmt.Sprintf("💬 Chat with AI  —  Model: %s", m.modelDisplay())
	b.WriteString(m.styles.header.Render(headerText))
	b.WriteByte('\n')

	// Messages area
	b.WriteString(m.viewport.View())
	b.WriteByte('\n')

	// Input area — use shell style when in ! mode
	inputStyle := m.styles.inputBox
	if strings.HasPrefix(m.input.Value(), "!") {
		inputStyle = m.styles.shellInputBox
	}
	inputView := inputStyle.Width(m.width - 4).Render(m.input.View())
	b.WriteString(inputView)
	b.WriteByte('\n')

	// Status bar
	status := m.renderStatus()
	b.WriteString(m.styles.statusBar.Width(m.width - 2).Render(status))

	return b.String()
}

func (m *chatModel) modelDisplay() string {
	if m.model != "" {
		return m.model
	}
	return "<API default>"
}

// ---------------------------------------------------------------------------
// Render helpers
// ---------------------------------------------------------------------------

// renderMessages returns the rendered viewport content from stored messages.
func (m *chatModel) renderMessages() string {
	if len(m.messages) == 0 {
		return "  Welcome! Type a message to start chatting.\n  • /help for available commands\n  • Ctrl+C or /exit to quit\n"
	}
	var b strings.Builder
	for _, msg := range m.messages {
		b.WriteString(m.renderOneMessage(msg))
		b.WriteByte('\n')
	}
	return b.String()
}

// renderMessagesWithStream returns rendered messages plus any in-progress
// streaming content.
func (m *chatModel) renderMessagesWithStream() string {
	var b strings.Builder
	for _, msg := range m.messages {
		b.WriteString(m.renderOneMessage(msg))
		b.WriteByte('\n')
	}
	// Append streaming content if any
	if m.streamBuf.Len() > 0 {
		b.WriteString(m.styles.asstMsg.Render("Assistant:"))
		b.WriteByte('\n')
		b.WriteString(m.streamBuf.String())
		b.WriteByte('\n')
	}
	return b.String()
}

func (m *chatModel) renderOneMessage(msg message) string {
	switch msg.role {
	case "user":
		return m.styles.userMsg.Render("You:") + "\n" + msg.content
	case "assistant":
		return m.styles.asstMsg.Render("Assistant:") + "\n" + msg.content
	case "system":
		return m.styles.sysMsg.Render(msg.content)
	case "tool":
		if msg.tool != "" {
			return m.styles.toolMsg.Render("🛠️  "+msg.tool+":") + "\n" + msg.content
		}
		return m.styles.toolMsg.Render(msg.content)
	default:
		return msg.content
	}
}

// renderStatus builds the status bar text.
func (m *chatModel) renderStatus() string {
	modelInfo := m.modelDisplay()
	switch m.state {
	case tuiIdle:
		if m.err != nil {
			return fmt.Sprintf("⚠️  Error: %v  |  Model: %s  |  F1: Help  |  Ctrl+C: Quit", m.err, modelInfo)
		}
		return fmt.Sprintf("● Ready  |  Model: %s  |  F1: Help  |  Ctrl+C: Quit", modelInfo)
	case tuiProcessing:
		return m.spinner.View() + " Processing…  |  Esc: Cancel"
	case tuiToolCall:
		return m.spinner.View() + " " + m.toolMsg + "  |  Esc: Cancel"
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// Tab completion
// ---------------------------------------------------------------------------

// handleTabCompletion completes commands (/xxx) or shell commands (!xxx).
func (m *chatModel) handleTabCompletion() (tea.Model, tea.Cmd) {
	if m.state != tuiIdle {
		return m, nil
	}

	input := m.input.Value()
	if input == "" {
		return m, nil
	}

	// Shell command completion: !<cmd>
	if strings.HasPrefix(input, "!") {
		return m.completeShellCmd(input)
	}

	// Command completion: /<cmd>
	return m.completeCommand(input)
}

// completeCommand completes /-prefixed commands.
func (m *chatModel) completeCommand(input string) (tea.Model, tea.Cmd) {
	// Reset cycle if input changed since last Tab
	if m.cycleIdx < 0 || len(m.cycleMatches) == 0 || !strings.HasPrefix(strings.ToLower(m.cycleMatches[m.cycleIdx]), strings.ToLower(input)) {
		m.cycleMatches = nil
		for _, c := range m.completions {
			if strings.HasPrefix(strings.ToLower(c), strings.ToLower(input)) {
				m.cycleMatches = append(m.cycleMatches, c)
			}
		}
		m.cycleIdx = -1
	}

	if len(m.cycleMatches) == 0 {
		return m, nil
	}

	m.cycleIdx = (m.cycleIdx + 1) % len(m.cycleMatches)
	m.input.SetValue(m.cycleMatches[m.cycleIdx])
	m.input.CursorEnd()
	return m, nil
}

// completeShellCmd completes executables in PATH for !-prefixed input.
func (m *chatModel) completeShellCmd(input string) (tea.Model, tea.Cmd) {
	prefix := strings.TrimPrefix(input, "!")

	// Reset cycle if input changed since last Tab
	if m.cycleIdx < 0 || len(m.cycleMatches) == 0 || !strings.HasPrefix(strings.ToLower(m.cycleMatches[m.cycleIdx]), strings.ToLower(prefix)) {
		m.cycleMatches = nil
		m.cycleIdx = -1

		// List executables from PATH
		pathDirs := filepath.SplitList(os.Getenv("PATH"))
		seen := make(map[string]bool)
		for _, dir := range pathDirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() || seen[name] {
					continue
				}
				if prefix == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
					m.cycleMatches = append(m.cycleMatches, "!"+name)
					seen[name] = true
				}
			}
		}
	}

	if len(m.cycleMatches) == 0 {
		return m, nil
	}

	m.cycleIdx = (m.cycleIdx + 1) % len(m.cycleMatches)
	m.input.SetValue(m.cycleMatches[m.cycleIdx])
	m.input.CursorEnd()
	return m, nil
}

// ---------------------------------------------------------------------------
// Help & tools display
// ---------------------------------------------------------------------------

func (m *chatModel) refreshViewport() {
	rendered := m.renderMessages()
	m.viewport.SetContent(rendered)
	m.viewport.GotoBottom()
}

func (m *chatModel) showHelp() {
	help := `Available commands:
  /exit, /quit, /q  Exit the chat
  /clear, /reset    Clear conversation history
  /help, /?         Show this help
  /tools            List available tools
  /<tool> <args>    Call a tool directly (e.g. /generate_image {"prompt":"a cat"})
  /preview <file>   Preview an image/video with system viewer
  !<command>        Run a shell command
  Ctrl+C/D          Quit
  Esc               Cancel current operation
  F1                Show this help`
	m.messages = append(m.messages, message{role: "system", content: help})
	m.refreshViewport()
}

func (m *chatModel) showTools() {
	if len(m.agentTools) == 0 {
		m.messages = append(m.messages, message{role: "system", content: "No tools available."})
		m.refreshViewport()
		return
	}
	var b strings.Builder
	b.WriteString("Available tools:\n")
	for _, t := range m.agentTools {
		fmt.Fprintf(&b, "  /%s", t.Function.Name)
		if desc := t.Function.Description; desc != "" {
			b.WriteString(" — " + desc)
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nUsage: /<tool_name> <json_args>")
	m.messages = append(m.messages, message{role: "system", content: b.String()})
	m.refreshViewport()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// send returns a tea.Cmd that sends a given message.
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

	// Initialize history with system prompt
	history := []types.ChatMessage{}
	if chatSystem != "" {
		history = append(history, types.ChatMessage{Role: "system", Content: chatSystem})
	}

	model := shared.Model
	if model == "" && chatCfg != nil {
		model = chatCfg.Model
	}

	// Build the TUI model
	tuiModel := newChatModel(c, agentTools, maxIterations, model, chatSystem, cmd, shared.Verbose,
		chatTemperature, chatMaxTokens)
	tuiModel.history = history

	// Create the Bubble Tea program with alt screen and signal handling
	// (tea.WithSignalHandler handles Ctrl+C properly, restoring terminal)
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
