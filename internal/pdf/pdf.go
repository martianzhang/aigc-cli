// Package pdf provides pure-Go PDF processing: text extraction for born-digital
// PDFs and embedded-image extraction for scanned documents. No CGO, no external
// CLI tools.
package pdf

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// PageText holds extracted text from one PDF page.
type PageText struct {
	Page int    // 1-based page number
	Text string // extracted plain text
}

// meaningfulChars returns the count of non-whitespace, non-numeric characters
// in s, used to determine whether a page has real text content.
func meaningfulChars(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) && !unicode.IsPunct(r) && !unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

// ExtractText opens a PDF and extracts plain text from every page.
// Returns one PageText per page. An empty Text means the page had no
// extractable content (typically a scanned/image page).
func ExtractText(path string) ([]PageText, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	numPages := r.NumPage()
	pages := make([]PageText, 0, numPages)

	for i := 1; i <= numPages; i++ {
		text := extractPageText(r.Page(i))
		pages = append(pages, PageText{Page: i, Text: text})
	}

	return pages, nil
}

// extractPageText extracts text from a single PDF page with paragraph
// structure preserved. Uses GetTextByRow() with Y-position thresholds
// to reconstruct paragraph breaks instead of GetPlainText() which
// concatenates all text in content-stream order without layout awareness.
func extractPageText(page pdf.Page) string {
	fonts := page.Fonts()
	fontMap := make(map[string]*pdf.Font, len(fonts))
	for _, name := range fonts {
		f := page.Font(name)
		fontMap[name] = &f
	}

	rows, err := page.GetTextByRow()
	if err != nil || len(rows) == 0 {
		// Fallback to GetPlainText on error or empty rows
		text, err := page.GetPlainText(fontMap)
		if err != nil {
			return ""
		}
		return strings.Join(strings.Fields(text), " ")
	}

	// Compute median absolute gap between rows for adaptive threshold.
	// PDF uses a downward Y-axis, so all gaps are negative; use Abs.
	var gaps []float64
	for i := 1; i < len(rows); i++ {
		g := math.Abs(float64(rows[i].Position - rows[i-1].Position))
		if g > 0.5 {
			gaps = append(gaps, g)
		}
	}

	threshold := 30.0
	if len(gaps) > 0 {
		sort.Float64s(gaps)
		median := gaps[len(gaps)/2]
		if median > 5 {
			threshold = median * 1.8
		}
	}

	var buf strings.Builder
	var prevPos *int64

	for _, row := range rows {
		if len(row.Content) == 0 {
			continue
		}

		rowText := extractRowText(row.Content)

		if prevPos != nil {
			gap := math.Abs(float64(row.Position - *prevPos))
			if gap > threshold {
				// Large gap: paragraph/section break.
				buf.WriteString("\n\n")
			} else if gap > 1 {
				// Line-spacing gap: new line.
				buf.WriteString("\n")
			}
		}

		buf.WriteString(rowText)
		prevPos = &row.Position
	}

	return buf.String()
}

// extractRowText concatenates text spans with line-break detection.
// Sorts spans by Y then X, and breaks lines at Y-position jumps (>0.5pt).
func extractRowText(spans pdf.TextHorizontal) string {
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Y != spans[j].Y {
			return spans[i].Y < spans[j].Y
		}
		return spans[i].X < spans[j].X
	})
	var buf strings.Builder
	prevY := spans[0].Y
	for _, t := range spans {
		s := strings.TrimSpace(t.S)
		if s == "" {
			continue
		}
		if t.Y-prevY > 0.5 {
			buf.WriteString("\n")
		}
		buf.WriteString(s)
		prevY = t.Y
	}
	return buf.String()
}

// IsScanned reports whether a PDF is a scanned/image document based on its
// extracted text. A PDF is considered scanned when every page has fewer than
// minChars meaningful (non-whitespace, non-punct, non-digit) characters.
const minCharsPerPage = 50

func IsScanned(pages []PageText) bool {
	if len(pages) == 0 {
		return true
	}
	for _, p := range pages {
		if meaningfulChars(p.Text) >= minCharsPerPage {
			return false
		}
	}
	return true
}
