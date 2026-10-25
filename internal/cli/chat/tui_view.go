package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

// chatMarkdownStyles is glamour's dark style with the document/code-block
// margin disabled. The message block already supplies the left bar and the
// indentation, so keeping glamour's margin would indent the body twice.
var chatMarkdownStyles = func() ansi.StyleConfig {
	s := styles.DarkStyleConfig
	s.Document.Margin = nil
	s.CodeBlock.Margin = nil
	return s
}()

func (m *chatModel) View() string {
	if !m.ready {
		return "\n  Initializing…"
	}

	var b strings.Builder

	// Header
	headerText := clipString(fmt.Sprintf("💬 Chat with AI  —  Model: %s", m.modelDisplay()), max(m.width-4, 20))
	b.WriteString(m.styles.header.Render(headerText))
	b.WriteByte('\n')

	// Messages area
	b.WriteString(m.viewport.View())
	b.WriteByte('\n')

	// Input area — use shell style when in ! mode
	var inputView string
	if m.pending != nil {
		inputView = m.renderConfirmBox()
	} else {
		inputStyle := m.styles.inputBox
		if strings.HasPrefix(m.input.Value(), "!") {
			inputStyle = m.styles.shellInputBox
		}
		inputWidth := max(m.width-4, 10)
		text, _ := m.inputBlock()
		inputView = inputStyle.Width(inputWidth).Render(text)
	}

	// Separator above the input box. Without it the transcript and the input
	// run together once the messages fill the screen.
	b.WriteString(m.styles.separator.Render(strings.Repeat("─", max(m.width-2, 10))))
	b.WriteByte('\n')

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
	body := strings.TrimSpace(m.streamBuf.String())
	if body == "" {
		return m.msgCache
	}
	// Reformatting markdown on every token would be far too slow, so the live
	// preview keeps the raw text. It still goes through the same message block,
	// though, so the label, the left bar and the indentation do not jump when
	// the turn finishes.
	preview := m.renderMessageBlock(m.styles.asstBlock, m.styles.asstMsg.Render("Assistant:"), body)
	if m.msgCache == "" {
		return preview
	}
	return m.msgCache + "\n" + preview
}

// renderMarkdown renders markdown to terminal-styled text via glamour.
// Skips rendering for plain text (no markdown syntax) to avoid extra whitespace.
func (m *chatModel) renderMarkdown(text string) string {
	// Model output regularly starts or ends with blank lines. Trimming here
	// (for plain text too) keeps them from showing up as empty rows under the
	// role label or before the next message.
	text = strings.TrimSpace(text)
	if text == "" || !containsMarkdown(text) {
		return text
	}
	// Wrap to the message area instead of glamour's fixed 80-column default.
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(chatMarkdownStyles),
		glamour.WithWordWrap(max(m.width-8, 20)),
	)
	if err != nil {
		return text
	}
	rendered, err := renderer.Render(text)
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
	switch msg.role {
	case "user":
		return m.renderMessageBlock(m.styles.userBlock, m.styles.userMsg.Render("You:"), msg.content)
	case "assistant":
		return m.renderMessageBlock(m.styles.asstBlock, m.styles.asstMsg.Render("Assistant:"), m.renderMarkdown(msg.content))
	case "system":
		return m.styles.sysMsg.Render(strings.TrimRight(msg.content, "\n"))
	case "sources":
		return m.styles.sourcesMsg.Render(strings.TrimRight(msg.content, "\n"))
	case "tool":
		if msg.tool == "" {
			return m.styles.toolMsg.Render("┃") + " " + msg.content
		}
		return m.renderToolMessage(msg)
	default:
		return msg.content
	}
}

// renderMessageBlock draws a role label and its body inside a coloured left
// bar. Wrapping the whole block — instead of prefixing just the label line —
// keeps the highlight running down the full height of the message and gives
// multi-line bodies a hanging indent.
func (m *chatModel) renderMessageBlock(block lipgloss.Style, label, body string) string {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return block.Render(label)
	}
	return block.Render(label + "\n" + wrapANSI(body, m.bodyWidth()))
}

// bodyWidth is the number of columns available for message text: the terminal
// width minus the viewport padding and the "┃ " prefix of the message block.
func (m *chatModel) bodyWidth() int {
	if m.width <= 0 {
		return 0
	}
	return max(m.width-6, 16)
}

// wrapANSI hard-wraps s to width display columns, breaking long runs that have
// no word boundary (CJK text, URLs). Escape sequences are copied through and
// the active SGR style is re-applied after every inserted line break, so
// colours survive the wrap (glamour's own word wrap only breaks at spaces).
func wrapANSI(s string, width int) string {
	if width < 1 {
		return s
	}
	var (
		out    strings.Builder
		active strings.Builder // SGR state to restore after a line break
		col    int
		state  byte
		parser = xansi.NewParser()
	)
	for i := 0; i < len(s); {
		seq, w, n, newState := xansi.DecodeSequence(s[i:], state, parser)
		if n <= 0 {
			out.WriteByte(s[i])
			i++
			continue
		}
		i += n
		state = newState

		switch {
		case seq == "\n":
			if active.Len() > 0 {
				out.WriteString("\x1b[0m\n")
				out.WriteString(active.String())
			} else {
				out.WriteByte('\n')
			}
			col = 0
		case strings.HasPrefix(seq, "\x1b["):
			out.WriteString(seq)
			if strings.HasSuffix(seq, "m") {
				if seq == "\x1b[0m" || seq == "\x1b[m" {
					active.Reset()
				} else {
					active.WriteString(seq)
				}
			}
		default:
			if w > 0 && col > 0 && col+w > width {
				if active.Len() > 0 {
					out.WriteString("\x1b[0m\n")
					out.WriteString(active.String())
				} else {
					out.WriteByte('\n')
				}
				col = 0
			}
			out.WriteString(seq)
			col += w
		}
	}
	return out.String()
}

// renderStatus builds the status bar text. Idle and confirmation states are
// clipped to the terminal width, so a long error message cannot wrap and break
// the layout. Spinner states are left alone because their ANSI escapes would
// confuse the rune-based clipper.
func (m *chatModel) renderStatus() string {
	info := m.providerModelDisplay()
	// The status bar style has one column of horizontal padding on each side,
	// so clip below its full width to keep the text from wrapping onto a
	// second row on narrow terminals.
	width := max(m.width-4, 20)
	switch m.state {
	case tuiIdle:
		if m.err != nil {
			return clipString(fmt.Sprintf("⚠️  Error: %v  |  %s  |  F1: Help  |  Ctrl+C: Quit", m.err, info), width)
		}
		return clipString(fmt.Sprintf("● Ready  |  %s  |  F1: Help  |  Ctrl+C: Quit", info), width)
	case tuiProcessing:
		if m.compacting {
			return m.spinner.View() + " Compacting…  |  Esc: Cancel"
		}
		return m.spinner.View() + " Processing…  |  Esc: Cancel"
	case tuiToolCall:
		return m.spinner.View() + " " + m.toolMsg + "  |  Esc: Cancel"
	case tuiConfirming:
		if m.pending != nil {
			return clipString(fmt.Sprintf("⚠️  Confirm %s  |  y: once  a: always  n: deny  |  Esc: deny & stop", m.pending.name), width)
		}
		return "⚠️  Waiting for confirmation…"
	default:
		return ""
	}
}

// renderConfirmBox replaces the input area while a tool call is awaiting
// approval. The tool card above already shows the command and diff.
func (m *chatModel) renderConfirmBox() string {
	p := m.pending
	width := max(m.width-6, 20)
	lines := []string{
		m.styles.toolMsg.Render("┃") + " " + m.styles.errMsg.Bold(true).Render("Confirm "+p.name),
	}

	if p.detail != "" {
		// Full command for bash: the card header is single-line, so show the
		// script here before asking for approval.
		cmdLines := strings.Split(strings.TrimRight(p.detail, "\n"), "\n")
		const maxCmdLines = 8
		for i, line := range cmdLines {
			if i >= maxCmdLines {
				lines = append(lines, "  "+m.styles.sysMsg.Render(
					fmt.Sprintf("… %d more lines (see tool card)", len(cmdLines)-maxCmdLines)))
				break
			}
			prefix := "    "
			if i == 0 {
				prefix = "  $ "
			}
			lines = append(lines, prefix+m.styles.toolArg.Render(clipString(line, width)))
		}
	} else if p.summary != "" {
		lines = append(lines, "  "+m.styles.toolArg.Render(clipString(p.summary, width)))
	}

	lines = append(lines, "  "+m.styles.sysMsg.Render(
		"[y] allow once   [a] allow for this session   [n] deny   Esc: deny & stop"))
	return m.styles.shellInputBox.Width(max(m.width-4, 10)).Render(strings.Join(lines, "\n"))
}

// ---------------------------------------------------------------------------
// Tab completion
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

// maxInputLines is the tallest the input box gets before the textarea scrolls
// internally.
const maxInputLines = 5

// inputPadding is a zero-width rune the textarea draws on the empty rows below
// a short draft. It is never visible; inputBlock uses it to tell those padding
// rows apart from a blank line that is really part of the draft.
const inputPadding = '\u200b'

// inputBlock returns the visible input box content and its row count. The
// textarea is kept at a fixed height so its own scroll position stays correct;
// the rows below the draft are trimmed here instead.
func (m *chatModel) inputBlock() (string, int) {
	lines := strings.Split(m.input.View(), "\n")
	rows := len(lines)
	for i, line := range lines {
		if strings.ContainsRune(line, inputPadding) {
			rows = i
			break
		}
	}
	rows = max(rows, 1)
	return strings.Join(lines[:rows], "\n"), rows
}

// syncLayout keeps the chrome in step with the draft: the input box shows as
// many rows as the draft needs (up to maxInputLines) and the message viewport
// takes the rest. While a tool confirmation is pending, the confirm box takes
// the input box's place and its own height is used.
func (m *chatModel) syncLayout() {
	if m.pending != nil {
		m.inputRows = strings.Count(m.renderConfirmBox(), "\n") + 1
	} else {
		_, m.inputRows = m.inputBlock()
	}
	if m.ready {
		m.viewport.Height = m.viewportHeight()
	}
}

// viewportHeight is the number of rows left for the transcript after the
// header, the input separator, the input box and the status bar.
func (m *chatModel) viewportHeight() int {
	rows := max(m.inputRows, 1)
	return max(m.height-rows-3, 3)
}

func (m *chatModel) refreshViewport() {
	// Preserve the scroll position when the user has scrolled up to read, and
	// follow new content only when they are already at the bottom.
	atBottom := m.viewport.AtBottom()
	rendered := m.renderMessages()
	m.viewport.SetContent(rendered)
	if atBottom {
		m.viewport.GotoBottom()
	}
}
