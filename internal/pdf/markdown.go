package pdf

import (
	"strings"
)

// BlocksToMarkdown converts DocumentBlocks to GitHub-Flavored Markdown.
func BlocksToMarkdown(blocks []DocumentBlock) string {
	if len(blocks) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, block := range blocks {
		if i > 0 && block.Type == BlockHeading {
			sb.WriteString("\n\n")
		} else if i > 0 && block.Type == BlockParagraph {
			sb.WriteString("\n")
		}

		switch block.Type {
		case BlockHeading:
			prefix := strings.Repeat("#", block.Level)
			sb.WriteString(prefix)
			sb.WriteByte(' ')
			sb.WriteString(applyInlineStyle(block.Content, block.Style))

		case BlockList:
			sb.WriteString(applyInlineStyle(block.Content, block.Style))

		case BlockParagraph:
			sb.WriteString(applyInlineStyle(block.Content, block.Style))

		default:
			sb.WriteString(block.Content)
		}
	}

	result := sb.String()
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(result) + "\n"
}

// applyInlineStyle wraps text with markdown formatting.
func applyInlineStyle(text string, style InlineStyle) string {
	if !style.Bold && !style.Italic {
		return text
	}

	result := text

	if style.Bold {
		result = "**" + result + "**"
	}

	if style.Italic {
		result = "*" + result + "*"
	}

	return result
}
