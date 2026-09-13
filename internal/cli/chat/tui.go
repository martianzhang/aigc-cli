package chat

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

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
// history carries the complete accumulated history from the goroutine;
// the main thread replaces m.history with it (fixes tool_calls + tool result loss).
type agentDone struct {
	result       *types.ChatResponse
	elapsed      time.Duration
	err          error
	assistantMsg *types.ChatMessage
	history      []types.ChatMessage // complete history from goroutine (replaces m.history)
}

// clearChat signals clearing the conversation.
type clearChat struct{}

// compactDone is sent when the conversation has been compacted.
type compactDone struct {
	summary    string
	err        error
	newHistory []types.ChatMessage // new history built by the goroutine
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
	contextSize int

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
