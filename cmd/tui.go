// Package cmd — Bubble Tea TUI for interactive chat.
//
// This file implements a Model-View-Update (MVU) chat interface using
// github.com/charmbracelet/bubbletea. It replaces the old raw-terminal
// input loop (runInteractiveChat) with a modern TUI experience.

package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/martianzhang/apimart-cli/internal/client"
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
	result       *types.ChatResponse
	elapsed      time.Duration
	err          error
	assistantMsg *types.ChatMessage
}

// clearChat signals clearing the conversation.
type clearChat struct{}

// compactDone is sent when the conversation has been compacted.
type compactDone struct {
	summary string
	err     error
}

// message
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

	// msgCache caches the rendered messages (without streaming) so we don't
	// re-render all messages on every streamChunk.
	msgCache string

	// compacting is true while the /compact goroutine is running.
	compacting bool

	// busy prevents new user input while an agent loop is in progress.
	busy bool

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
	// 999 effectively removes the width cap — real width is set via SetWidth on resize
	ti.MaxWidth = 999
	ti.MaxHeight = 5
	// Minimal style — no cursor line highlight
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()

	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#335577", Dark: "#66AAFF"})
	s.Spinner = spinner.Dot

	// Build completion list from tool definitions
	completions := []string{"/exit", "/quit", "/q", "/clear", "/reset", "/new", "/help", "/?", "/tools", "/copy"}
	for _, t := range tools {
		completions = append(completions, "/"+t.Function.Name)
	}

	// Pre-fill with welcome message so renderMessages always renders messages
	welcomeMsg := buildWelcomeBanner()

	return chatModel{
		state:       tuiIdle,
		client:      c,
		agentTools:  tools,
		maxIters:    maxIt,
		model:       mdl,
		cmd:         cobraCmd,
		verbose:     verb,
		temperature: temp,
		maxTokens:   maxTok,
		input:       ti,
		spinner:     s,
		messages:    []message{{role: "system", content: welcomeMsg}},
		completions: completions,
		cycleIdx:    -1,
		histIdx:     -1,
		streamBuf:   &strings.Builder{},
		mu:          &sync.Mutex{},
		system:      sys, // store original for /clear; date hint in history separately
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
		// Reserve at least 6 lines for chrome (header + input box + status bar + padding)
		chromeHeight := 6
		vpHeight := msg.Height - chromeHeight
		if vpHeight < 3 {
			vpHeight = 3
		}
		if !m.ready {
			m.viewport = viewport.New(msg.Width-2, vpHeight)
			m.viewport.YPosition = 1
			m.viewport.Style = m.styles.messages
			m.viewport.SetContent(m.renderMessages())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 2
			m.viewport.Height = vpHeight
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
		m.busy = false
		// Append assistant response to history (from the goroutine via msg)
		if msg.assistantMsg != nil {
			m.history = append(m.history, *msg.assistantMsg)
		}
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

	// ---- compact conversation -------------------------------------------
	case compactDone:
		m.state = tuiIdle
		m.compacting = false
		m.busy = false
		m.input.Focus()
		m.input.Reset()

		if msg.err != nil {
			m.err = msg.err
			m.messages = append(m.messages, message{
				role:    "system",
				content: fmt.Sprintf("Compact failed: %v", msg.err),
			})
			m.refreshViewport()
			return m, nil
		}

		// Replace TUI messages with the compacted summary
		m.messages = []message{
			{role: "system", content: "Compacted conversation into summary below."},
			{role: "system", content: msg.summary},
		}
		m.refreshViewport()
		return m, nil

	// ---- clear chat ------------------------------------------------------
	case clearChat:
		m.messages = []message{{role: "system", content: buildWelcomeBanner()}}
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
