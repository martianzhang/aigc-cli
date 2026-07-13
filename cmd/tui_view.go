package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbletea"
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

// buildWelcomeBanner returns the startup welcome screen text.
func buildWelcomeBanner() string {
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#55DD99"))
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("#DDCC44"))
	gray := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	a := lipgloss.NewStyle().Foreground(lipgloss.Color("#66DDEE"))
	i := lipgloss.NewStyle().Foreground(lipgloss.Color("#55DD99"))
	g := lipgloss.NewStyle().Foreground(lipgloss.Color("#DDCC44"))
	c := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8866"))

	b := strings.Builder{}
	b.WriteByte('\n')
	b.WriteString(a.Render("      █████") + "  " + i.Render(" ██") + "  " + g.Render(" ██████   ") + "  " + c.Render("   ██████ "))
	b.WriteString("\n")
	b.WriteString(a.Render("    ██   ██") + "  " + i.Render(" ██") + "  " + g.Render("██        ") + "  " + c.Render(" ██       "))
	b.WriteString("\n")
	b.WriteString(a.Render("   ████████") + "  " + i.Render(" ██") + "  " + g.Render("██     ███") + "  " + c.Render("██        "))
	b.WriteString("\n")
	b.WriteString(a.Render("  ██     ██") + "  " + i.Render(" ██") + "  " + g.Render("██      ██") + "  " + c.Render(" ██       "))
	b.WriteString("\n")
	b.WriteString(a.Render(" ██      ██") + "  " + i.Render(" ██") + "  " + g.Render("  ██████  ") + "  " + c.Render("   ██████ "))
	b.WriteString("\n")

	b.WriteString(gray.Render("  ──────────────────────────────────────"))
	b.WriteString("\n")
	b.WriteString(green.Render("  💬  Type a message to start chatting"))
	b.WriteString("\n")
	b.WriteString(yellow.Render("  ⌨️  /help  —  commands & shortcuts"))
	b.WriteString("\n")
	b.WriteString(gray.Render("  🚀  Tab complete  ·  Ctrl+C quit"))
	b.WriteString("\n")
	b.WriteString(gray.Render("  ──────────────────────────────────────"))
	b.WriteString("\n")

	return b.String()
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
  /clear, /reset, /new  Clear conversation history
  /compact             Compact conversation (summarize to save context)
  /copy                Copy last assistant response to clipboard
  /help, /?            Show this help
  /tools               List available tools
  /<tool> <args>       Call a tool directly (e.g. /generate_image {"prompt":"a cat"})
  /preview <file>      Preview an image/video with system viewer
  !<command>           Run a shell command
  Ctrl+C/D             Quit
  Esc                  Cancel current operation
  F1                   Show this help`
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

// handleCopy copies the last assistant response to the system clipboard.
func (m *chatModel) handleCopy() {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].role == "assistant" && m.messages[i].content != "" {
			if err := clipboard.WriteAll(m.messages[i].content); err != nil {
				m.messages = append(m.messages, message{role: "system", content: fmt.Sprintf("Copy failed: %v", err)})
			} else {
				m.messages = append(m.messages, message{role: "system", content: "\u2713 Last response copied to clipboard"})
			}
			m.refreshViewport()
			return
		}
	}
	m.messages = append(m.messages, message{role: "system", content: "Nothing to copy \u2014 no assistant response found."})
	m.refreshViewport()
}
