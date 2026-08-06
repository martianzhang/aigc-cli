package pdf

import (
	"math"
	"regexp"
	"strings"

	gopdf "github.com/razvandimescu/gopdf/pdf"
)

// lineGroup represents a group of TextSpans on the same visual line.
type lineGroup struct {
	spans []gopdf.TextSpan
	y     float64
}

// analyzeLayout extracts document structure from TextSpans using heuristic rules.
// It detects headings, lists, and inline formatting (bold/italic).
func analyzeLayout(spans []gopdf.TextSpan) []DocumentBlock {
	if len(spans) == 0 {
		return nil
	}

	// Calculate body font size (mode of all font sizes)
	bodyFontSize := calcBodyFontSize(spans)

	// Group spans by Y coordinate into lines
	lines := groupByY(spans, 8.0)

	// Analyze each line
	var blocks []DocumentBlock
	for _, line := range lines {
		text := joinLine(line.spans)
		if strings.TrimSpace(text) == "" {
			continue
		}

		fontSize := dominantFontSize(line.spans)
		font := dominantFont(line.spans)
		style := InlineStyle{
			Bold:   isBoldFont(font),
			Italic: isItalicFont(font),
		}

		if fontSize > bodyFontSize*1.2 {
			level := headingLevel(fontSize, bodyFontSize)
			blocks = append(blocks, DocumentBlock{
				Type:    BlockHeading,
				Level:   level,
				Content: text,
				Style:   style,
			})
			continue
		}

		if isListPrefix(text) {
			blocks = append(blocks, DocumentBlock{
				Type:    BlockList,
				Content: formatListItem(text),
				Style:   style,
			})
			continue
		}

		blocks = append(blocks, DocumentBlock{
			Type:    BlockParagraph,
			Content: text,
			Style:   style,
		})
	}

	return blocks
}

// calcBodyFontSize calculates the body font size using mode (most frequent).
func calcBodyFontSize(spans []gopdf.TextSpan) float64 {
	freq := make(map[int]int)
	for _, s := range spans {
		// Round to 0.5pt for grouping
		bucket := int(s.FontSize * 2)
		freq[bucket]++
	}

	maxCount := 0
	maxBucket := 0
	for bucket, count := range freq {
		if count > maxCount {
			maxCount = count
			maxBucket = bucket
		}
	}

	return float64(maxBucket) / 2.0
}

// groupByY groups TextSpans into lines based on Y coordinate proximity.
// PDF coordinates are bottom-up (Y=0 at bottom, Y=height at top).
func groupByY(spans []gopdf.TextSpan, tolerance float64) []lineGroup {
	if len(spans) == 0 {
		return nil
	}

	// Sort by Y descending (top to bottom in visual order)
	sorted := make([]gopdf.TextSpan, len(spans))
	copy(sorted, spans)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Y > sorted[i].Y {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var lines []lineGroup
	currentLine := lineGroup{
		spans: []gopdf.TextSpan{sorted[0]},
		y:     sorted[0].Y,
	}

	for i := 1; i < len(sorted); i++ {
		if math.Abs(sorted[i].Y-currentLine.y) <= tolerance {
			currentLine.spans = append(currentLine.spans, sorted[i])
		} else {
			sortByX(currentLine.spans)
			lines = append(lines, currentLine)
			currentLine = lineGroup{
				spans: []gopdf.TextSpan{sorted[i]},
				y:     sorted[i].Y,
			}
		}
	}
	sortByX(currentLine.spans)
	lines = append(lines, currentLine)

	return lines
}

// sortByX sorts TextSpans by X coordinate (left to right).
func sortByX(spans []gopdf.TextSpan) {
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			if spans[j].X < spans[i].X {
				spans[i], spans[j] = spans[j], spans[i]
			}
		}
	}
}

// joinLine joins TextSpans into a single string with appropriate spacing.
func joinLine(spans []gopdf.TextSpan) string {
	if len(spans) == 0 {
		return ""
	}

	var result strings.Builder
	for i, span := range spans {
		text := strings.TrimSpace(span.Text)
		if text == "" {
			continue
		}

		if i > 0 {
			prev := spans[i-1]
			prevText := strings.TrimSpace(prev.Text)
			if prevText == "" {
				result.WriteByte(' ')
			}
		}
		result.WriteString(text)
	}

	return result.String()
}

// dominantFontSize returns the font size with the most characters.
func dominantFontSize(spans []gopdf.TextSpan) float64 {
	freq := make(map[int]int) // bucket = fontSize * 2
	for _, s := range spans {
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}
		bucket := int(s.FontSize * 2)
		freq[bucket] += len([]rune(text))
	}

	maxCount := 0
	maxBucket := 0
	for bucket, count := range freq {
		if count > maxCount {
			maxCount = count
			maxBucket = bucket
		}
	}

	if maxCount == 0 {
		return 12.0 // default
	}
	return float64(maxBucket) / 2.0
}

// dominantFont returns the font name with the most characters.
func dominantFont(spans []gopdf.TextSpan) string {
	freq := make(map[string]int)
	for _, s := range spans {
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}
		freq[s.Font] += len([]rune(text))
	}

	maxCount := 0
	maxFont := ""
	for font, count := range freq {
		if count > maxCount {
			maxCount = count
			maxFont = font
		}
	}

	return maxFont
}

// isBoldFont checks if font name indicates bold.
func isBoldFont(font string) bool {
	lower := strings.ToLower(font)
	return strings.Contains(lower, "bold") ||
		strings.Contains(lower, "black") ||
		strings.Contains(lower, "heavy") ||
		strings.Contains(lower, "semibold") ||
		strings.Contains(lower, "demibold")
}

// isItalicFont checks if font name indicates italic.
func isItalicFont(font string) bool {
	lower := strings.ToLower(font)
	return strings.Contains(lower, "italic") ||
		strings.Contains(lower, "oblique") ||
		strings.Contains(lower, "slant")
}

// isListPrefix checks if text starts with a list marker.
func isListPrefix(text string) bool {
	trimmed := strings.TrimSpace(text)

	// Bullet markers
	bullets := []string{"• ", "● ", "○ ", "◦ ", "- ", "* "}
	for _, b := range bullets {
		if strings.HasPrefix(trimmed, b) {
			return true
		}
	}

	// Numbered list: "1.", "1)", "(1)"
	if matched, _ := regexp.MatchString(`^\d+[\.\)]\s`, trimmed); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^\(\d+\)\s`, trimmed); matched {
		return true
	}

	// Letter list: "a.", "a)", "(a)"
	if matched, _ := regexp.MatchString(`^[a-zA-Z][\.\)]\s`, trimmed); matched {
		return true
	}
	if matched, _ := regexp.MatchString(`^\([a-zA-Z]\)\s`, trimmed); matched {
		return true
	}

	return false
}

// formatListItem normalizes list markers to markdown format.
func formatListItem(text string) string {
	trimmed := strings.TrimSpace(text)

	// Convert bullet characters to "-"
	for _, bullet := range []string{"•", "●", "○", "◦"} {
		if strings.HasPrefix(trimmed, bullet+" ") {
			return "- " + trimmed[len(bullet)+1:]
		}
		if strings.HasPrefix(trimmed, "**"+bullet) {
			// Bold bullet: "**• Label:** rest"
			return "- " + trimmed
		}
	}

	// Already markdown format
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		return trimmed
	}

	// Numbered/letter lists: keep as-is (markdown supports them)
	return trimmed
}
