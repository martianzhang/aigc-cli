package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

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
	inputWidth := m.width - 4
	if inputWidth < 10 {
		inputWidth = 10
	}
	inputView := inputStyle.Width(inputWidth).Render(m.input.View())
	b.WriteString(inputView)
	b.WriteByte('\n')

	// Status bar
	status := m.renderStatus()
	b.WriteString(m.styles.statusBar.Width(m.width - 2).Render(status))

	return b.String()
}

// renderMessages returns the rendered viewport content from stored messages.
// Updates msgCache so streamChunk rendering can skip re-rendering all messages.
func (m *chatModel) renderMessages() string {
	var b strings.Builder
	for i, msg := range m.messages {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.renderOneMessage(msg))
	}
	m.msgCache = b.String()
	return m.msgCache
}

// renderMessagesWithStream returns cached messages plus any in-progress
// streaming content — much faster than re-rendering all messages on every chunk.
func (m *chatModel) renderMessagesWithStream() string {
	if m.streamBuf.Len() == 0 {
		return m.msgCache
	}
	return m.msgCache + "\n" + m.styles.asstMsg.Render("Assistant:") + "\n" + m.streamBuf.String()
}

// renderMarkdown renders markdown to terminal-styled text via glamour.
// Skips rendering for plain text (no markdown syntax) to avoid extra whitespace.
func (m *chatModel) renderMarkdown(text string) string {
	// Only use glamour if the text contains markdown syntax
	if !containsMarkdown(text) {
		return text
	}
	rendered, err := glamour.Render(text, "dark")
	if err != nil {
		return text
	}
	return strings.TrimSpace(rendered)
}

// containsMarkdown reports whether text likely contains markdown formatting.
func containsMarkdown(s string) bool {
	return strings.Contains(s, "**") || strings.Contains(s, "##") ||
		strings.Contains(s, "`") || strings.Contains(s, "*") ||
		strings.Contains(s, "---") || strings.Contains(s, "[") && strings.Contains(s, "](") ||
		strings.HasPrefix(s, "#") || strings.HasPrefix(s, ">") ||
		strings.HasPrefix(s, "-") || strings.HasPrefix(s, "1.")
}

func (m *chatModel) renderOneMessage(msg message) string {
	const indent = "  "
	switch msg.role {
	case "user":
		// blue left bracket + bold label
		header := lipgloss.NewStyle().Foreground(lipgloss.Color("#66AAFF")).Render("┃")
		label := m.styles.userMsg.Render("You:")
		return header + " " + label + "\n" + indent + msg.content
	case "assistant":
		// green left bracket + green label
		header := lipgloss.NewStyle().Foreground(lipgloss.Color("#55DD99")).Render("┃")
		label := m.styles.asstMsg.Render("Assistant:")
		rendered := m.renderMarkdown(msg.content)
		return header + " " + label + "\n" + indent + rendered
	case "system":
		return m.styles.sysMsg.Render(msg.content)
	case "tool":
		header := m.styles.toolMsg.Render("┃")
		if msg.tool != "" {
			return header + " " + m.styles.toolMsg.Render("🛠️  "+msg.tool+":") + "\n" + indent + msg.content
		}
		return header + " " + msg.content
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
		if m.compacting {
			return m.spinner.View() + " Compacting…  |  Esc: Cancel"
		}
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

func (m *chatModel) refreshViewport() {
	rendered := m.renderMessages()
	m.viewport.SetContent(rendered)
	m.viewport.GotoBottom()
}
