package chat

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// pushHistory adds a non-empty command to history, deduplicating the last entry.
// compactConversation sends the full history to the API for summarisation
// and sends a compactDone message back to the TUI with the result.
func (m *chatModel) compactConversation(ctx context.Context, turn uint64, count int) {
	prog := getProgram()

	m.mu.Lock()
	history := make([]types.ChatMessage, len(m.history))
	copy(history, m.history)
	m.mu.Unlock()

	if len(history) == 0 {
		prog.Send(compactDone{turn: turn, err: fmt.Errorf("no conversation to compact")})
		return
	}

	result := agent.CompactConversation(m.client.WithContext(ctx), m.model, m.temperature, m.maxTokens, history, &logWriter{prog: prog, turn: turn})
	if result == nil {
		prog.Send(compactDone{turn: turn, err: fmt.Errorf("compaction failed")})
		return
	}

	prog.Send(compactDone{
		turn:       turn,
		summary:    fmt.Sprintf("Compacted %d messages into 1 summary:\n\n%s", count, result.Summary),
		newHistory: result.History,
	})
}

func (m *chatModel) handleUserMessage(input string) (tea.Model, tea.Cmd) {
	// Resolve @file mentions: text files inline, images become attachments.
	expanded, images, warnings := expandMentions(input, readDenyPatterns())

	// Remove initial welcome message on first real message
	if len(m.messages) == 1 && m.messages[0].role == "system" {
		m.messages = nil
	}
	// Display the text the user typed (not the inlined expansion).
	m.messages = append(m.messages, message{role: "user", content: input})
	for _, warning := range warnings {
		m.messages = append(m.messages, message{role: "system", content: "⚠️  " + warning})
	}

	// Add to API history
	m.mu.Lock()
	m.history = append(m.history, types.ChatMessage{Role: "user", Content: expanded, Images: images})
	m.mu.Unlock()
	m.input.Reset()
	m.input.Blur()

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
