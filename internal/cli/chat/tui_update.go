package chat

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/types"
)

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

	// ---- mouse wheel scrolls the message viewport ------------------------
	case tea.MouseMsg:
		// Without this the wheel falls through to the terminal, which scrolls the
		// main-screen scrollback and reveals the shell history behind the alt
		// screen. Forwarding to the viewport keeps scrolling inside the chat.
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	// ---- streaming -------------------------------------------------------
	case streamChunk:
		m.streamBuf.WriteString(strings.ReplaceAll(string(msg), "\r", ""))
		rendered := m.renderMessagesWithStream()
		// Only stick to the bottom when the user is already there, so scrolling
		// up to read during streaming is not immediately yanked back down.
		atBottom := m.viewport.AtBottom()
		m.viewport.SetContent(rendered)
		if atBottom {
			m.viewport.GotoBottom()
		}
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
		for i, tm := range slices.Backward(m.messages) {
			if tm.role == "tool" && tm.tool == msg.name {
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
		// Replace history with the goroutine's accumulated snapshot.
		// This is the ONLY path by which history is updated: the goroutine
		// never writes m.history directly (it works on a local copy).
		if msg.history != nil {
			m.history = msg.history
		} else if msg.assistantMsg != nil {
			// Fallback for direct calls that may not carry full history
			m.history = append(m.history, *msg.assistantMsg)
		}
		// Flush stream buffer into a proper message
		if m.streamBuf.Len() > 0 {
			m.messages = append(m.messages, message{
				role:    "assistant",
				content: strings.ReplaceAll(m.streamBuf.String(), "\r", ""),
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
		// Replace history with the goroutine's compacted version
		if msg.newHistory != nil {
			m.history = msg.newHistory
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
