package cmd

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/lipgloss"
)

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

func (m *chatModel) modelDisplay() string {
	if m.model != "" {
		return m.model
	}
	return "<API default>"
}

// ---------------------------------------------------------------------------
// Render helpers
// ---------------------------------------------------------------------------

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
  PgUp/PgDn            Scroll conversation output
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
