package docconvert

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/martianzhang/aigc-cli/internal/pdf"
)

// fromPDF extracts the text layer and converts it to Markdown. Scanned PDFs
// return ErrNoTextLayer and structurally unusual PDFs return ErrComplexLayout;
// preview opens both in the system viewer instead of guessing.
func fromPDF(path string) (string, error) {
	if pdf.IsEncrypted(path) {
		return "", fmt.Errorf("%s is encrypted; open it with a PDF viewer", path)
	}
	blocks, err := pdf.ExtractTextWithLayout(path)
	if err != nil {
		return "", fmt.Errorf("extract PDF text: %w", err)
	}
	markdown := strings.TrimSpace(pdf.BlocksToMarkdown(blocks))
	if markdown == "" {
		return "", ErrNoTextLayer
	}

	issues := pdf.LayoutIssues(path)
	if suspiciousText(markdown) {
		issues = append(issues, "unreadable characters")
	}
	if len(issues) > 0 {
		return "", fmt.Errorf("%w: %s", ErrComplexLayout, strings.Join(issues, ", "))
	}
	return markdown + "\n", nil
}

// suspiciousText reports text whose glyphs did not map to usable Unicode:
// replacement characters, private-use glyphs and control characters.
func suspiciousText(s string) bool {
	total, bad := 0, 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		switch {
		case r == unicode.ReplacementChar:
			bad++
		case r >= 0xE000 && r <= 0xF8FF:
			bad++
		case unicode.IsControl(r):
			bad++
		}
	}
	return total >= 100 && bad >= 5 && bad*50 >= total
}
