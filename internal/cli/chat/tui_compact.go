package chat

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// compactConversation sends the older part of the history to the API for
// summarisation and sends a compactDone message back to the TUI with the
// result. The recent tail is kept verbatim; the on-screen transcript is
// preserved and a divider is appended instead of being wiped.
func (m *chatModel) compactConversation(ctx context.Context, turn uint64, count int, instructions string) {
	prog := getProgram()

	m.mu.Lock()
	history := make([]types.ChatMessage, len(m.history))
	copy(history, m.history)
	anchor := m.usageAnchor
	m.mu.Unlock()

	if len(history) == 0 {
		prog.Send(compactDone{turn: turn, err: fmt.Errorf("no conversation to compact")})
		return
	}

	toolsTokens := m.toolsTokens
	if m.rt != nil {
		toolsTokens = agent.EstimateToolDefinitions(m.rt.RequestTools()) + agent.EstimateRequestExtras(chatInjectRaw)
	}
	lw := &logWriter{prog: prog, turn: turn}
	opts := compactionOptions(m.compactSettings, anchor, toolsTokens, m.sess, m.model, "manual", true, lw)
	opts.CustomInstructions = instructions

	result, err := agent.Compact(ctx, m.client.WithContext(ctx), history, opts)
	if err != nil {
		prog.Send(compactDone{turn: turn, err: err})
		return
	}
	if err := persistCompaction(m.sess, result, m.model); err != nil {
		prog.Send(compactDone{turn: turn, err: err})
		return
	}

	prog.Send(compactDone{
		turn:           turn,
		count:          count,
		kept:           result.KeepLast,
		summary:        result.Summary,
		newHistory:     result.History,
		lastCompaction: result,
		contextTokens:  result.Tokens,
	})
}

// appendUserMessage records what the user typed in the transcript. The welcome
// banner is left in place so the logo and the shortcut hints stay visible in
// the scrollback for the whole session.
func (m *chatModel) appendUserMessage(input string, warnings []string) {
	// Display the text the user typed (not the inlined expansion).
	m.messages = append(m.messages, message{role: "user", content: input})
	for _, warning := range warnings {
		m.messages = append(m.messages, message{role: "system", content: "⚠️  " + warning})
	}
}

func (m *chatModel) handleUserMessage(input string) (tea.Model, tea.Cmd) {
	// Resolve @file mentions: text files inline, images become attachments.
	expanded, images, warnings := expandMentions(input, readDenyPatterns())
	return m.startUserTurn(input, expanded, images, warnings)
}

// startUserTurn appends one user message to the transcript and starts the
// agent loop. display is what the user typed; expanded is what the model
// receives (after @file or /skill: expansion).
func (m *chatModel) startUserTurn(display, expanded string, images []types.ChatImage, warnings []string) (tea.Model, tea.Cmd) {
	m.appendUserMessage(display, warnings)

	// Add to API history
	userMessage := types.ChatMessage{Role: "user", Content: expanded, Images: images}
	m.mu.Lock()
	m.history = append(m.history, userMessage)
	m.mu.Unlock()
	if m.sess != nil {
		if err := m.sess.AppendMessage(userMessage, "", m.toolOutputMax); err != nil {
			m.messages = append(m.messages, message{role: "system", content: fmt.Sprintf("Session write failed: %v", err)})
		}
	}
	// The input stays focused and empty so the next message can be drafted
	// while the reply streams in; Enter is blocked until the turn finishes.
	m.input.Reset()

	// Switch state
	m.state = tuiProcessing
	m.busy = true
	m.streamBuf.Reset()
	m.err = nil

	// Each turn gets a fresh cancellable context so Esc can stop the HTTP
	// request and any tool it is running, and a turn id so late messages from
	// an interrupted turn are dropped.
	m.turnID++
	turn := m.turnID
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx, m.cancel = ctx, cancel

	// Render user message immediately
	m.refreshViewport()

	// Start agent loop in a goroutine
	go m.runTUIAgentLoop(ctx, turn)

	return m, m.spinner.Tick
}
