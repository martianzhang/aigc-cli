// Package mdrender renders Markdown to terminal-styled text. It is shared by
// the chat TUI and `aigc-cli preview` so both show the same output.
package mdrender

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	glamourstyles "github.com/charmbracelet/glamour/styles"
	xansi "github.com/charmbracelet/x/ansi"
)

// terminalStyles is glamour's dark style with the document/code-block margin
// disabled. Callers already supply their own left bar and indentation, so
// keeping glamour's margin would indent the body twice.
var terminalStyles = func() ansi.StyleConfig {
	styles := glamourstyles.DarkStyleConfig
	styles.Document.Margin = nil
	styles.CodeBlock.Margin = nil
	return styles
}()

// Render renders Markdown for a content area of contentWidth columns. Prose is
// rendered wide and wrapped later with WrapANSI (glamour pads every line to its
// wrap width and its own wrapping cannot be undone); tables are rendered at
// their own width so narrow tables stay narrow.
func Render(src string, contentWidth int) string {
	// Model output and documents regularly start or end with blank lines;
	// trimming here keeps them from showing up as empty rows.
	text := strings.TrimSpace(src)
	if text == "" || !containsMarkdown(text) {
		return text
	}
	segments := splitMarkdownTables(text)
	if len(segments) == 1 {
		return renderBlock(segments[0], contentWidth)
	}
	var b strings.Builder
	for _, seg := range segments {
		rendered := renderBlock(seg, contentWidth)
		if rendered == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(rendered)
	}
	if b.Len() == 0 {
		return text
	}
	return b.String()
}

// renderBlock renders one segment at the width its content needs.
func renderBlock(seg mdSegment, contentWidth int) string {
	if seg.table {
		return renderMarkdownTable(seg.src, tableWidth(contentWidth))
	}
	width := proseWidth(contentWidth)
	if containsTableStart(seg.src) {
		// A table that could not be lifted out (indented into a list item, or
		// glued to a paragraph) still needs the content width, otherwise
		// glamour spreads it over the prose width.
		width = tableWidth(contentWidth)
	}
	return renderSegment(seg.src, width)
}

// proseWidth is the word-wrap width used for prose. It is deliberately far
// wider than the content area so glamour leaves the wrapping to WrapANSI.
func proseWidth(contentWidth int) int {
	return max(contentWidth*4, 512)
}

// tableWidth is the widest a rendered table may be before it is clamped to the
// content area.
func tableWidth(contentWidth int) int {
	return max(contentWidth, 20)
}

// renderSegment renders one block of Markdown at the given word-wrap width and
// strips the padding glamour adds after every line.
func renderSegment(src string, wrapWidth int) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(terminalStyles),
		glamour.WithWordWrap(wrapWidth),
	)
	if err != nil {
		return src
	}
	rendered, err := renderer.Render(src)
	if err != nil {
		return src
	}
	return trimMarkdownPadding(rendered)
}

// renderMarkdownTable renders one GFM table block. glamour spreads a table over
// its whole word-wrap width, so rendering a narrow table at the content width
// leaves a wide gap between the columns — a two-column table becomes two
// islands. Word wrap 0 makes lipgloss fall back to the natural, content-based
// column widths; only a table wider than the content area is rendered at that
// width instead, so its cells wrap rather than overflow.
func renderMarkdownTable(src string, width int) string {
	natural := renderSegment(src, 0)
	if width <= 0 || widestLine(natural) <= width {
		return natural
	}
	return renderSegment(src, width)
}

// widestLine returns the width in display columns of the widest line of s,
// ignoring escape sequences.
func widestLine(s string) int {
	widest := 0
	for _, line := range strings.Split(s, "\n") {
		widest = max(widest, xansi.StringWidth(line))
	}
	return widest
}

// mdSegment is one slice of a document: either a GFM table block or ordinary
// Markdown.
type mdSegment struct {
	src   string
	table bool
}

// splitMarkdownTables cuts s into alternating prose and table segments. Tables
// are pulled out because they are rendered at their own width. Fenced code
// blocks are left intact so their contents are never mistaken for a table.
func splitMarkdownTables(s string) []mdSegment {
	lines := strings.Split(s, "\n")
	var (
		segs    []mdSegment
		cur     []string
		inFence bool
	)
	flush := func(table bool) {
		if len(cur) == 0 {
			return
		}
		segs = append(segs, mdSegment{src: strings.Join(cur, "\n"), table: table})
		cur = nil
	}
	for i := 0; i < len(lines); i++ {
		if isCodeFence(lines[i]) {
			inFence = !inFence
			cur = append(cur, lines[i])
			continue
		}
		if inFence || !isTableStart(lines, i) {
			cur = append(cur, lines[i])
			continue
		}
		if strings.TrimLeft(lines[i], " \t") != lines[i] {
			// Indented: the table belongs to a list item, so it cannot be
			// rendered without its parent block.
			cur = append(cur, lines[i])
			continue
		}
		j := i + 2 // skip the header and the delimiter row
		for j < len(lines) && strings.Contains(lines[j], "|") && strings.TrimSpace(lines[j]) != "" {
			j++
		}
		// Only lift a table out when it stands on its own: one glued to a
		// paragraph would be re-parsed out of context once the paragraph is
		// rendered without it.
		if (i > 0 && strings.TrimSpace(lines[i-1]) != "") || (j < len(lines) && strings.TrimSpace(lines[j]) != "") {
			cur = append(cur, lines[i:j]...)
			i = j - 1
			continue
		}
		flush(false)
		cur = append(cur, lines[i:j]...)
		flush(true)
		i = j - 1
	}
	flush(false)
	return segs
}

// isTableStart reports whether lines[i] starts a GFM table: a header row with a
// pipe followed by a delimiter row.
func isTableStart(lines []string, i int) bool {
	if i+1 >= len(lines) || !strings.Contains(lines[i], "|") {
		return false
	}
	return strings.Count(lines[i+1], "|") >= 2 && isTableDelimiter(lines[i+1])
}

// containsTableStart reports whether s starts a GFM table anywhere outside a
// fenced code block.
func containsTableStart(s string) bool {
	lines := strings.Split(s, "\n")
	inFence := false
	for i := range lines {
		if isCodeFence(lines[i]) {
			inFence = !inFence
			continue
		}
		if !inFence && isTableStart(lines, i) {
			return true
		}
	}
	return false
}

// isCodeFence reports whether line opens or closes a fenced code block.
func isCodeFence(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

// isTableDelimiter reports whether line looks like "|---|:--:|".
func isTableDelimiter(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "-") {
		return false
	}
	for _, r := range line {
		switch r {
		case '|', '-', ':', ' ', '\t':
		default:
			return false
		}
	}
	return true
}

// trimMarkdownPadding removes the blank rows and the styled padding cells that
// glamour writes at the end of each line (it pads to its wrap width). Leading
// spaces are indentation and must survive: glamour spreads a table's first row
// across its columns with them.
func trimMarkdownPadding(rendered string) string {
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		lines[i] = trimLineRight(line)
	}
	start, end := 0, len(lines)
	for start < end && lines[start] == "" {
		start++
	}
	for end > start && lines[end-1] == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

// trimLineRight drops trailing spaces from a styled line without breaking the
// escape sequences: it keeps everything up to the last visible non-space cell
// and closes any style that was still open.
func trimLineRight(s string) string {
	var (
		state byte
		p     = xansi.NewParser()
		last  int
	)
	for i := 0; i < len(s); {
		seq, w, n, newState := xansi.DecodeSequence(s[i:], state, p)
		if n <= 0 {
			seq, w, n = s[i:i+1], 1, 1
		}
		i += n
		state = newState
		if w > 0 && seq != " " {
			last = i
		}
	}
	if last == 0 {
		return ""
	}
	if last == len(s) {
		return s
	}
	return s[:last] + "\x1b[0m"
}

// containsMarkdown reports whether text likely contains markdown formatting.
func containsMarkdown(s string) bool {
	return strings.Contains(s, "**") || strings.Contains(s, "##") ||
		strings.Contains(s, "`") || strings.Contains(s, "*") ||
		strings.Contains(s, "---") || strings.Contains(s, "[") && strings.Contains(s, "](") ||
		strings.HasPrefix(s, "#") || strings.HasPrefix(s, ">") ||
		strings.HasPrefix(s, "-") || strings.HasPrefix(s, "1.")
}
