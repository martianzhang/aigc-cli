package agent

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	defaultToolResultSummaryChars = 2000
	defaultToolArgsSummaryChars   = 500
)

// truncateForSummary shortens a string at a rune boundary and appends a marker
// explaining how much was omitted.
func truncateForSummary(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	omitted := len(s) - cut
	return s[:cut] + "\n\n[... " + strconv.Itoa(omitted) + " more bytes truncated]"
}

// serializeConversation renders messages as plain text for summarization. It
// deliberately does not send a chat transcript back to the model: a text log
// prevents the summarizer from treating the input as a conversation to
// continue. Tool results and tool arguments are truncated; images become
// placeholders so base64 never reaches the summary request.
func serializeConversation(messages []types.ChatMessage, toolResultChars, toolArgChars int) string {
	if toolResultChars <= 0 {
		toolResultChars = defaultToolResultSummaryChars
	}
	if toolArgChars <= 0 {
		toolArgChars = defaultToolArgsSummaryChars
	}

	var parts []string
	for _, m := range messages {
		switch m.Role {
		case "system":
			continue
		case "user":
			text := m.Content
			if text != "" {
				parts = append(parts, "[User]: "+text)
			}
			for _, img := range m.Images {
				name := img.Name
				if name == "" {
					name = "image"
				}
				parts = append(parts, "[Image]: "+name)
			}
		case "assistant":
			if m.Content != "" {
				parts = append(parts, "[Assistant]: "+m.Content)
			}
			for _, tc := range m.ToolCalls {
				args := truncateForSummary(tc.Function.Arguments, toolArgChars)
				parts = append(parts, "[Assistant tool calls]: "+tc.Function.Name+"("+args+")")
			}
		case "tool":
			if m.Content != "" {
				parts = append(parts, "[Tool result]: "+truncateForSummary(m.Content, toolResultChars))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}
