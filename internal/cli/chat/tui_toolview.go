package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/martianzhang/aigc-cli/internal/agent"
)

const (
	// toolOutputPreviewLines is how many lines of a tool result are shown before
	// folding. Ctrl+O or a click expands the most recent tool card.
	toolOutputPreviewLines = 20
	// toolDiffMaxLines caps a rendered diff so a large write cannot flood the
	// viewport.
	toolDiffMaxLines = 40
	// toolDiffMaxFileBytes caps the existing file read used to diff a write.
	toolDiffMaxFileBytes = 512 << 10
	// toolSummaryWidth caps the one-line argument summary in the tool header.
	toolSummaryWidth = 100
)

// toolCallSummary renders a short, single-line description of a tool call's
// arguments for the tool header. Unknown (MCP) tools fall back to compacted
// JSON. The result never contains newlines.
func toolCallSummary(name, argsJSON string) string {
	args := parseToolArgs(argsJSON)
	switch name {
	case "bash":
		cmd := firstLine(argString(args, "command"))
		if cmd == "" {
			return ""
		}
		return clipString("$ "+cmd, toolSummaryWidth)
	case "read":
		s := argString(args, "filepath")
		if s == "" {
			return ""
		}
		var page []string
		if v := argString(args, "offset"); v != "" && v != "0" {
			page = append(page, "offset "+v)
		}
		if v := argString(args, "limit"); v != "" && v != "0" {
			page = append(page, "limit "+v)
		}
		if len(page) > 0 {
			s += " (" + strings.Join(page, ", ") + ")"
		}
		return clipString(s, toolSummaryWidth)
	case "write":
		path := argString(args, "filepath")
		if path == "" {
			return ""
		}
		return clipString(fmt.Sprintf("%s (+%d lines)", path, lineCount(argString(args, "content"))), toolSummaryWidth)
	case "edit":
		path := argString(args, "filepath")
		oldText := argString(args, "old_string")
		newText := argString(args, "new_string")
		if path == "" {
			return ""
		}
		return clipString(fmt.Sprintf("%s (-%d/+%d lines)",
			path, lineCount(oldText), lineCount(newText)), toolSummaryWidth)
	case "grep":
		pattern := argString(args, "pattern")
		if pattern == "" {
			return ""
		}
		s := "/" + firstLine(pattern) + "/"
		if inc := argString(args, "include"); inc != "" {
			s += " " + inc
		}
		if path := argString(args, "path"); path != "" {
			s += " in " + path
		}
		return clipString(s, toolSummaryWidth)
	case "find":
		pattern := argString(args, "pattern")
		if pattern == "" {
			return ""
		}
		s := pattern
		if path := argString(args, "path"); path != "" {
			s += " in " + path
		}
		return clipString(s, toolSummaryWidth)
	case "ls":
		if path := argString(args, "path"); path != "" {
			return clipString(path, toolSummaryWidth)
		}
		return "."
	case "search", "web_search":
		if q := argString(args, "query"); q != "" {
			return clipString(q, toolSummaryWidth)
		}
		return ""
	case "fetch", "web_fetch":
		if u := argString(args, "url"); u != "" {
			return clipString(u, toolSummaryWidth)
		}
		return ""
	}

	// Unknown / MCP tool: show compacted JSON, omitting an empty object.
	compact := compactJSON(argsJSON)
	if compact == "" || compact == "{}" {
		return ""
	}
	return clipString(compact, toolSummaryWidth)
}

// toolCallDiff returns a +/- diff preview for write and edit calls. An empty
// string means the tool has no useful diff (or the content is unchanged).
func toolCallDiff(name, argsJSON string) string {
	args := parseToolArgs(argsJSON)
	switch name {
	case "edit":
		oldText := argString(args, "old_string")
		newText := argString(args, "new_string")
		if oldText == "" && newText == "" {
			return ""
		}
		return lineDiff(oldText, newText, 0, toolDiffMaxLines)
	case "write":
		content := argString(args, "content")
		if content == "" {
			return ""
		}
		oldText, _ := readTextCapped(argString(args, "filepath"), toolDiffMaxFileBytes)
		return lineDiff(oldText, content, 2, toolDiffMaxLines)
	default:
		return ""
	}
}

// lineDiff renders a compact block diff: common prefix/suffix lines are
// trimmed, the middle is shown as removals followed by additions, and at most
// maxLines output lines are emitted. context adds unchanged lines around the
// change.
func lineDiff(oldText, newText string, context, maxLines int) string {
	if oldText == newText {
		return ""
	}
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}

	var out []string
	truncated := false
	add := func(prefixChar, line string) {
		if len(out) >= maxLines {
			truncated = true
			return
		}
		out = append(out, prefixChar+line)
	}

	if context > 0 {
		for i := max(0, prefix-context); i < prefix; i++ {
			add(" ", oldLines[i])
		}
	}
	for _, line := range oldLines[prefix : len(oldLines)-suffix] {
		add("-", line)
	}
	for _, line := range newLines[prefix : len(newLines)-suffix] {
		add("+", line)
	}
	if context > 0 {
		tail := len(newLines) - suffix
		for i := tail; i < len(newLines) && i < tail+context; i++ {
			add(" ", newLines[i])
		}
	}
	if truncated {
		out = append(out, "…")
	}
	return strings.Join(out, "\n")
}

// splitLines splits text into lines without a trailing empty element. Empty
// input yields no lines (so a brand-new file diffs as pure additions).
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// lineCount returns the number of lines in a snippet, 0 for empty input.
func lineCount(text string) int {
	return len(splitLines(text))
}

// readTextCapped reads at most limit bytes and refuses binary or oversized
// files. ok is false when no usable text could be read.
func readTextCapped(path string, limit int64) (string, bool) {
	if path == "" {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(f, limit+1))
	if err != nil || n > limit {
		return "", false
	}
	data := buf.Bytes()
	if agent.LooksBinary(data) {
		return "", false
	}
	return string(data), true
}

// parseToolArgs decodes a tool argument object. Invalid or non-object JSON
// yields an empty map; callers fall back to compactJSON for display.
func parseToolArgs(argsJSON string) map[string]any {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil
	}
	return args
}

// argString returns a scalar argument as a string, or "" when absent.
func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// JSON numbers: avoid "1.000000" for integers.
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%v", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return ""
	}
}

// compactJSON collapses all whitespace so a JSON argument object fits on one
// display line.
func compactJSON(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// firstLine returns the text before the first CR/LF.
func firstLine(s string) string {
	if idx := strings.IndexAny(s, "\r\n"); idx >= 0 {
		return s[:idx]
	}
	return s
}

// clipString truncates s to maxWidth display columns, appending an ellipsis.
// It verifies the measured width of the result, which keeps emoji and other
// wide clusters from overshooting by one column.
func clipString(s string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 {
		candidate := string(runes) + "…"
		if lipgloss.Width(candidate) <= maxWidth {
			return candidate
		}
		runes = runes[:len(runes)-1]
	}
	return "…"
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// renderToolMessage renders a tool message: header + argument summary, an
// optional full command and diff when expanded, then the (possibly folded)
// result.
func (m *chatModel) renderToolMessage(msg message) string {
	var b strings.Builder

	header := m.styles.toolMsg.Render("┃") + " " + m.styles.toolMsg.Render("🛠️  "+msg.tool)
	// When expanded, a bash card shows the full command below instead of the
	// clipped one-line summary, so do not print the summary twice.
	showDetail := msg.expanded && msg.toolDetail != ""
	if msg.toolSummary != "" && !showDetail {
		header += " " + m.styles.toolArg.Render(msg.toolSummary)
	}
	b.WriteString(header)

	if showDetail {
		b.WriteByte('\n')
		b.WriteString(m.renderToolDetail(msg.toolDetail, max(m.width-4, 20)))
	}

	if msg.toolDiff != "" {
		b.WriteByte('\n')
		b.WriteString(m.renderDiff(msg.toolDiff, max(m.width-4, 20)))
	}

	lines := splitLines(strings.TrimRight(msg.content, "\n"))
	total := len(lines)
	folded := !msg.expanded && total > toolOutputPreviewLines
	if folded {
		lines = lines[:toolOutputPreviewLines]
	}
	for _, line := range lines {
		b.WriteString("\n  ")
		b.WriteString(clipString(line, max(m.width-4, 20)))
	}

	switch {
	case folded && commandAbbreviated(msg):
		fmt.Fprintf(&b, "\n  %s", m.styles.sysMsg.Render(fmt.Sprintf(
			"… command abbreviated, %d more lines (click or Ctrl+O to expand)", total-toolOutputPreviewLines)))
	case folded:
		fmt.Fprintf(&b, "\n  %s", m.styles.sysMsg.Render(fmt.Sprintf(
			"… %d more lines (click or Ctrl+O to expand)", total-toolOutputPreviewLines)))
	case commandAbbreviated(msg):
		fmt.Fprintf(&b, "\n  %s", m.styles.sysMsg.Render("… command abbreviated (click or Ctrl+O to expand)"))
	case msg.expanded && (showDetail || total > toolOutputPreviewLines):
		fmt.Fprintf(&b, "\n  %s", m.styles.sysMsg.Render("(click or Ctrl+O to collapse)"))
	}
	return b.String()
}

// commandAbbreviated reports whether the collapsed header is hiding part of
// the tool's argument detail (a clipped or multi-line bash command).
func commandAbbreviated(msg message) bool {
	detail := strings.TrimRight(msg.toolDetail, "\r\n")
	return !msg.expanded && detail != "" && msg.toolSummary != "$ "+detail
}

// renderToolDetail renders the full argument text shown when a bash card is
// expanded. Only the first row carries the "$ " prompt; continuation lines and
// wrapped rows are indented so the command reads as one block.
func (m *chatModel) renderToolDetail(detail string, width int) string {
	lines := strings.Split(strings.TrimRight(detail, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		wrapped, _ := wrapANSI(line, max(width-4, 10))
		for j, row := range strings.Split(wrapped, "\n") {
			prefix := "    "
			if i == 0 && j == 0 {
				prefix = "  $ "
			}
			out = append(out, prefix+m.styles.toolArg.Render(row))
		}
	}
	return strings.Join(out, "\n")
}

// renderDiff colorises a pre-rendered diff string.
func (m *chatModel) renderDiff(diff string, width int) string {
	lines := strings.Split(diff, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = clipString(line, width)
		switch {
		case strings.HasPrefix(line, "+"):
			out = append(out, "  "+m.styles.diffAdd.Render(line))
		case strings.HasPrefix(line, "-"):
			out = append(out, "  "+m.styles.diffDel.Render(line))
		default:
			out = append(out, "  "+m.styles.diffCtx.Render(line))
		}
	}
	return strings.Join(out, "\n")
}
