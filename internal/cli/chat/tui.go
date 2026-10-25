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
	tuiConfirming                 // waiting for the user to approve a tool call
)

// ---------------------------------------------------------------------------
// Tea message types
// ---------------------------------------------------------------------------

// streamChunk is sent for each streaming content delta from the API.
// turn identifies the agent turn that produced it: after the user interrupts,
// late chunks from the cancelled turn are dropped instead of leaking into the
// next response.
type streamChunk struct {
	turn uint64
	text string
}

// logMsg is sent for status/log output during tool execution (e.g. config info).
type logMsg struct {
	turn uint64
	text string
}

// toolStart is sent when a tool call begins.
type toolStart struct {
	turn    uint64
	name    string
	summary string // one-line argument summary for the tool header
	diff    string // optional pre-rendered diff (write/edit)
}

// toolDone is sent when a tool call completes.
type toolDone struct {
	turn    uint64
	name    string // tool name
	summary string // one-line summary for status bar
	content string // full output for the message area (empty = use summary)
}

// agentDone is sent when the agent loop finishes for one user turn.
// history carries the complete accumulated history from the goroutine;
// the main thread replaces m.history with it (fixes tool_calls + tool result loss).
type agentDone struct {
	turn         uint64
	result       *types.ChatResponse
	elapsed      time.Duration
	err          error
	assistantMsg *types.ChatMessage
	history      []types.ChatMessage // complete history from goroutine (replaces m.history)
}

// clearChat signals clearing the conversation.
type clearChat struct{}

// confirmRequest asks the UI to approve a privileged tool call. The agent
// goroutine blocks on reply (or on its context) until the user answers.
type confirmRequest struct {
	turn    uint64
	name    string
	summary string
	detail  string // full multi-line command (bash); empty for other tools
	reply   chan confirmDecision
}

// pendingConfirm is the UI-side view of a confirmRequest.
type pendingConfirm struct {
	name    string
	summary string
	detail  string
	reply   chan confirmDecision
}

// compactDone is sent when the conversation has been compacted.
type compactDone struct {
	turn       uint64
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

	// toolSummary is the one-line argument summary shown next to the tool name
	// (e.g. the bash command, or "file.go (-2/+5 lines)" for an edit).
	toolSummary string
	// toolDiff is a pre-rendered +/- diff for write/edit calls.
	toolDiff string
	// expanded controls whether a long tool output is shown in full. Ctrl+O
	// toggles it for the most recent tool message.
	expanded bool
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

	// turnID identifies the current async operation (agent turn or compact).
	// It is bumped whenever the user starts or interrupts one, so messages
	// from a cancelled turn can be recognised and dropped.
	turnID uint64

	// confirm decides which tool calls need user approval. nil disables
	// confirmation (used by tests and by non-TUI callers).
	confirm *confirmPolicy
	// pending is non-nil while the UI is waiting for a y/a/n answer.
	pending *pendingConfirm

	// Conversation history for API requests (types.ChatMessage, not TUI message)
	history []types.ChatMessage

	// Dependencies
	client     *client.Client
	rt         *toolRuntime
	agentTools []types.ToolDefinition
	maxIters   int
	model      string
	provider   string
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
	// fileIndex caches project files for @-mention completion (built lazily).
	fileIndex      []string
	fileIndexReady bool

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
	toolArg       lipgloss.Style
	diffAdd       lipgloss.Style
	diffDel       lipgloss.Style
	diffCtx       lipgloss.Style
	errMsg        lipgloss.Style
}
