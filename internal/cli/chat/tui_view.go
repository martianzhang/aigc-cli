package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/martianzhang/aigc-cli/internal/mdrender"
)

func (m *chatModel) View() string {
	m.syncTitle()
	if !m.ready {
		return "\n  Initializing…"
	}

	var b strings.Builder

	// Header — session name plus the working directory. The model is shown in
	// the status bar only, so it is not repeated here.
	b.WriteString(m.styles.header.Render(m.headerText()))
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
// Updates msgCache so streamChunk rendering can skip re-rendering all messages,
// and records which lines belong to which message (with the separator needed to
// rebuild soft-wrapped text) for copy-on-select.
func (m *chatModel) renderMessages() string {
	var b strings.Builder
	m.msgRanges = m.msgRanges[:0]
	line := 0
	for i, msg := range m.messages {
		if i > 0 {
			// The separator newline only terminates the previous message; the
			// next one starts on the following line, which is `line` already.
			b.WriteByte('\n')
		}
		rendered, seps := m.renderOneMessage(msg)
		m.msgRanges = append(m.msgRanges, msgRange{
			msg:   i,
			role:  msg.role,
			start: line,
			end:   line + len(seps),
			seps:  seps,
		})
		line += len(seps)
		b.WriteString(rendered)
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
	preview, _ := m.renderMessageBlock(m.styles.asstBlock, m.styles.asstMsg.Render("Assistant:"), body)
	if m.msgCache == "" {
		return preview
	}
	return m.msgCache + "\n" + preview
}

// renderMarkdown renders markdown to terminal-styled text via the shared
// renderer, so the TUI and `aigc-cli preview` show the same output.
func (m *chatModel) renderMarkdown(text string) string {
	return mdrender.Render(text, m.bodyWidth())
}

func (m *chatModel) renderOneMessage(msg message) (string, []string) {
	switch msg.role {
	case "user":
		return m.renderMessageBlock(m.styles.userBlock, m.styles.userMsg.Render("You:"), msg.content)
	case "assistant":
		return m.renderMessageBlock(m.styles.asstBlock, m.styles.asstMsg.Render("Assistant:"), m.renderMarkdown(msg.content))
	case "system":
		return hardLineBreaks(m.styles.sysMsg.Render(strings.TrimRight(msg.content, "\n")))
	case "sources":
		return hardLineBreaks(m.styles.sourcesMsg.Render(strings.TrimRight(msg.content, "\n")))
	case "tool":
		if msg.tool == "" {
			return hardLineBreaks(m.styles.toolMsg.Render("┃") + " " + msg.content)
		}
		return hardLineBreaks(m.renderToolMessage(msg))
	default:
		return hardLineBreaks(msg.content)
	}
}

// hardLineBreaks marks every line of s as separated by a real newline.
func hardLineBreaks(s string) (string, []string) {
	seps := make([]string, strings.Count(s, "\n")+1)
	for i := 1; i < len(seps); i++ {
		seps[i] = "\n"
	}
	return s, seps
}

// renderMessageBlock draws a role label and its body inside a coloured left
// bar. Wrapping the whole block — instead of prefixing just the label line —
// keeps the highlight running down the full height of the message and gives
// multi-line bodies a hanging indent. The returned separators let a copied
// selection undo wrapping instead of turning it into hard newlines.
func (m *chatModel) renderMessageBlock(block lipgloss.Style, label, body string) (string, []string) {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return block.Render(label), []string{""}
	}
	wrapped, joins := mdrender.WrapANSI(body, m.bodyWidth())
	seps := make([]string, 0, len(joins)+2)
	seps = append(seps, "", "\n")
	seps = append(seps, joins...)
	return block.Render(label + "\n" + wrapped), seps
}

// bodyWidth is the number of columns available for message text: the terminal
// width minus the viewport padding and the "┃ " prefix of the message block.
func (m *chatModel) bodyWidth() int {
	if m.width <= 0 {
		return 0
	}
	return max(m.width-6, 16)
}

// renderStatus builds the status bar text. Idle and confirmation states are
// clipped to the terminal width, so a long error message cannot wrap and break
// the layout. Spinner states are left alone because their ANSI escapes would
// confuse the rune-based clipper.
func (m *chatModel) renderStatus() string {
	info := m.providerModelDisplay()
	if pct := m.contextPercent(); pct >= 0 {
		info += fmt.Sprintf("  |  ctx %s%%", formatContextPercent(pct))
	}
	if rec := m.sessionDisplay(); rec != "" {
		info += "  |  " + rec
	}
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
			if m.suggesting {
				return clipString(fmt.Sprintf("✏️  Tell the model what to do instead  |  Enter: send  Esc: back  |  %s", m.pending.name), width)
			}
			return clipString(fmt.Sprintf("⚠️  Confirm %s  |  y: once  a: always  n: deny  s: suggest  |  Esc: deny & stop", m.pending.name), width)
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

	rows, hidden := confirmCommandRows(p)
	for i, line := range rows {
		prefix := "  "
		if p.detail != "" {
			prefix = "    "
			if i == 0 {
				prefix = "  $ "
			}
		}
		lines = append(lines, prefix+m.styles.toolArg.Render(clipString(line, width)))
	}
	if hidden > 0 {
		lines = append(lines, "  "+m.styles.sysMsg.Render(
			fmt.Sprintf("… %d more lines (click the tool card above to expand)", hidden)))
	}

	lines = append(lines, "  "+m.styles.sysMsg.Render(confirmPromptHint(m.suggesting)))
	if m.suggesting {
		if typed := strings.TrimRight(m.input.Value(), "\n"); typed != "" {
			for _, line := range strings.Split(typed, "\n") {
				lines = append(lines, "  "+m.styles.toolArg.Render(clipString(line, width)))
			}
		}
	}
	return m.styles.shellInputBox.Width(max(m.width-4, 10)).Render(strings.Join(lines, "\n"))
}

// confirmMaxCmdLines caps how many command rows the confirmation box shows; the
// rest stay available by clicking the tool card above.
const confirmMaxCmdLines = 8

// confirmCommandRows returns the command/summary rows shown in the confirmation
// box and how many trailing lines are hidden. It is shared by rendering and
// mouse hit-testing so the option row always lines up.
func confirmCommandRows(p *pendingConfirm) (rows []string, hidden int) {
	if p == nil {
		return nil, 0
	}
	if p.detail != "" {
		all := strings.Split(strings.TrimRight(p.detail, "\n"), "\n")
		if len(all) > confirmMaxCmdLines {
			return all[:confirmMaxCmdLines], len(all) - confirmMaxCmdLines
		}
		return all, 0
	}
	if p.summary != "" {
		return []string{p.summary}, 0
	}
	return nil, 0
}

// confirmChoice is one clickable option in the confirmation box. key is the
// keyboard shortcut it mirrors; 0 means Esc (deny and stop the turn).
const confirmChoiceSep = "   "

type confirmChoice struct {
	label string
	key   rune
}

var confirmChoices = []confirmChoice{
	{"[y] allow once", 'y'},
	{"[a] always", 'a'},
	{"[n] deny", 'n'},
	{"[s] suggest", 's'},
	{"[Esc] stop", 0},
}

// confirmPromptHint is the key legend at the bottom of the confirmation box.
func confirmPromptHint(suggesting bool) string {
	if suggesting {
		return "tell the model what to do instead — [Enter] send   [Esc] back   (empty Enter = deny)"
	}
	labels := make([]string, len(confirmChoices))
	for i, c := range confirmChoices {
		labels[i] = c.label
	}
	return strings.Join(labels, confirmChoiceSep)
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
	if m.input.Value() == "" {
		// The placeholder is longer than a narrow terminal and can wrap. An
		// empty draft must still occupy exactly one input row; keeping the
		// wrapped placeholder would grow the box after every command.
		rows = 1
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
