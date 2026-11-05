package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

	lines := []string{
		"",
		a.Render("    ___ ") + "  " + i.Render("    ____") + "  " + g.Render("    ______") + "  " + c.Render("   _____ "),
		a.Render("   /   |") + "  " + i.Render("   /  _/") + "  " + g.Render("  / _____/") + "  " + c.Render("  / ___/ "),
		a.Render("  / /| |") + "  " + i.Render("   / /  ") + "  " + g.Render(" / /  ___ ") + "  " + c.Render(" / /     "),
		a.Render(" / ___ |") + "  " + i.Render(" _/ /   ") + "  " + g.Render("/ /___/ / ") + "  " + c.Render("/ /___  "),
		a.Render("/_/  |_|") + "  " + i.Render("/___/   ") + "  " + g.Render("\\_____/  ") + "  " + c.Render(" \\____/ "),
		gray.Render("  ──────────────────────────────────────"),
		green.Render("  💬  Type a message to start chatting"),
		yellow.Render("  ⌨️  /help  —  commands & shortcuts"),
		gray.Render("  📎  @path  —  attach a file (Tab completes)"),
		gray.Render("  🚀  Tab complete  ·  Ctrl+C quit"),
		gray.Render("  ──────────────────────────────────────"),
	}

	return strings.Join(lines, "\n") + "\n"
}

func (m *chatModel) modelDisplay() string {
	if m.model != "" {
		return m.model
	}
	return "<API default>"
}

// headerText is the top bar: the session label followed by the working
// directory the agent runs in. The model is deliberately left out — the status
// bar already shows it, and repeating it just wasted a row of context.
//
// The directory keeps its tail when space runs out: the leaf folder says where
// the agent works, while the leading part of the path rarely does.
func (m *chatModel) headerText() string {
	label := "💬 " + m.sessionTitle()
	width := max(m.width-4, 20)
	dir := m.workDirDisplay()
	if dir == "" {
		return clipString(label, width)
	}
	const sep = "  ·  "
	budget := width - lipgloss.Width(label) - lipgloss.Width(sep)
	if budget <= 0 {
		return clipString(label, width)
	}
	return label + sep + clipPathLeft(dir, budget)
}

// clipPathLeft shortens a path from the left, keeping the trailing components:
// "~/ws/github.com/org/project" becomes "…/org/project" when it does not fit.
func clipPathLeft(path string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(path) <= maxWidth {
		return path
	}
	if maxWidth == 1 {
		return "…"
	}
	// Drop whole leading components while the remainder still fits. Split on
	// the portable slash so both separators behave the same.
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := 1; i < len(parts); i++ {
		rest := "…/" + strings.Join(parts[i:], "/")
		if lipgloss.Width(rest) <= maxWidth {
			// Always use forward slashes in the display — path separators in
			// the terminal header should not depend on the OS.
			return rest
		}
	}
	return clipString("…/"+strings.TrimPrefix(filepath.ToSlash(path), "/"), maxWidth)
}

// workDirDisplay is the working directory with $HOME collapsed to "~", so the
// top bar shows where relative paths, @-mentions and tool calls resolve.
func (m *chatModel) workDirDisplay() string {
	dir := m.workDir
	if dir == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if strings.HasPrefix(dir, home+string(os.PathSeparator)) {
		return "~" + dir[len(home):]
	}
	return dir
}

// providerModelDisplay renders the "Provider | Model" segment for the status bar.
func (m *chatModel) providerModelDisplay() string {
	model := "Model: " + m.modelDisplay()
	if m.provider != "" {
		return "Provider: " + m.provider + "  |  " + model
	}
	return model
}

// ---------------------------------------------------------------------------
// Render helpers
// ---------------------------------------------------------------------------

func (m *chatModel) showHelp() {
	help := `Available commands:
  /exit, /quit, /q  Exit the chat
  /clear, /reset, /new  Clear conversation history
  /model [name]        Show or switch the model (default = API default)
  /provider [name]     Show or switch the provider (default = CLI/config)
  /context             Show context usage, threshold and last compaction
  /compact [focus]     Compact now (optional focus for the summary)
  /copy                Copy last assistant response to clipboard
  /name [text]         Show/set the session name; no text = auto-name
  /save [name]         Save this chat as a session (or rename the current one)
  /session [n|id|path] List recent sessions or switch to one
  /session find <text> Search all sessions by name, id, first message or model
  /resume              Alias for /session
  /help, /?            Show this help
  /tools               List available tools
  /tools <name>        Show a tool's full description and examples
  /skills              List loaded skills and what they do
  /<tool> <args>       Call a tool directly (e.g. /generate_image {"prompt":"a cat"})
  /preview <file>      Preview an image/video with system viewer
  /skill:<name> [args] Load a skill explicitly (skills in .agents/skills)
  @path                Attach a file: text is inlined, images are sent as media
                       (Tab completes paths; defaults.chat.read_deny still applies)
  !<command>           Run a shell command
  Enter                Send the message
  Shift/Alt+Enter      Insert a line break (Ctrl+J works everywhere)
  Up/Down              History — or move the cursor inside a multi-line draft
  PgUp/PgDn or wheel  Scroll conversation output
  Ctrl+O               Expand/collapse the last tool card (command + output)
  Click a tool card    Toggle that card between abbreviated and full view
  Drag                 Select transcript rows and copy them without the frame
  Shift+drag           Native terminal selection — iTerm2: ⌥ Option + drag
  Ctrl+C               Interrupt the current turn; when idle, quit
  Ctrl+D               Quit (ignored while a draft is present)
  Esc                  Interrupt the current turn (partial output is kept)
  F1                   Show this help`
	m.messages = append(m.messages, message{role: "system", content: help})
	m.refreshViewport()
}

func (m *chatModel) showTools(name string) {
	if len(m.agentTools) == 0 {
		m.messages = append(m.messages, message{role: "system", content: "No tools available."})
		m.refreshViewport()
		return
	}

	if name != "" {
		m.showToolDetail(strings.TrimPrefix(name, "/"))
		return
	}

	var b strings.Builder
	b.WriteString("Available tools:\n")
	for _, t := range m.agentTools {
		fmt.Fprintf(&b, "  /%-20s  %s\n", t.Function.Name, collapsedSummary(t.Function.Description, 90))
	}
	b.WriteString("\nUsage: /<tool_name> <json_args>")
	b.WriteString("\nFull description: /tools <tool_name>")
	m.messages = append(m.messages, message{role: "system", content: b.String()})
	m.refreshViewport()
}

// showToolDetail prints one tool's complete description.
func (m *chatModel) showToolDetail(name string) {
	for _, t := range m.agentTools {
		if t.Function.Name != name {
			continue
		}
		desc := strings.TrimSpace(t.Function.Description)
		if desc == "" {
			desc = "(no description)"
		}
		m.messages = append(m.messages, message{role: "system", content: "/" + name + "\n\n" + desc})
		m.refreshViewport()
		return
	}
	m.messages = append(m.messages, message{
		role:    "system",
		content: fmt.Sprintf("Unknown tool %q. Run /tools for the list.", name),
	})
	m.refreshViewport()
}

// collapsedSummary reduces a tool description to a single line for the list
// view: the first non-empty line, inner whitespace collapsed, truncated to
// maxRunes.
//
// Tool descriptions are rich multi-line schema text — several MCP tools ship
// 13–24 lines with their own indentation and an Examples block. Inlining that
// after the tool name left every continuation line at column 0, so the list
// shows a one-line summary and /tools <name> prints the full text.
func collapsedSummary(desc string, maxRunes int) string {
	line := ""
	for _, ln := range strings.Split(desc, "\n") {
		if strings.TrimSpace(ln) != "" {
			line = ln
			break
		}
	}
	line = strings.Join(strings.Fields(line), " ")
	if line == "" {
		return "(no description)"
	}
	if runes := []rune(line); len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "…"
	}
	return line
}

// handleCopy copies the last assistant response to the system clipboard.
func (m *chatModel) handleCopy() {
	for _, msg := range slices.Backward(m.messages) {
		if msg.role == "assistant" && msg.content != "" {
			if err := clipboard.WriteAll(msg.content); err != nil {
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
