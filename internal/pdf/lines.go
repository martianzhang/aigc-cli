package pdf

import (
	"math"
	"sort"
	"unicode"

	gopdf "github.com/razvandimescu/gopdf/pdf"
)

// TextSegment is one text run inside a line. Coordinates are PDF points.
type TextSegment struct {
	Text string
	X0   float64
	Y0   float64
	X1   float64
	Y1   float64
	// Start/End are byte offsets in TextLine.Text.
	Start int
	End   int
}

// TextLine is one reconstructed line with a PDF-point bounding box. Y0/Y1 use
// PDF user-space coordinates (origin bottom-left).
type TextLine struct {
	Text     string
	X0       float64
	Y0       float64
	X1       float64
	Y1       float64
	Segments []TextSegment
}

// PageLines is positioned text for one PDF page.
type PageLines struct {
	Page     int
	OriginX  float64 // MediaBox left
	OriginY  float64 // MediaBox bottom
	Width    float64 // points
	Height   float64 // points
	Rotation int
	Lines    []TextLine
}

// ExtractPageLines extracts positioned lines from a PDF text layer. Pages that
// are scanned or rotated return no lines; callers should render and OCR those.
func ExtractPageLines(path string) ([]PageLines, error) {
	doc, err := gopdfOpen(path)
	if err != nil {
		return nil, err
	}
	numPages := doc.NumPages()
	out := make([]PageLines, 0, numPages)
	for i := 0; i < numPages; i++ {
		page := doc.Page(i)
		mb := page.MediaBox()
		pl := PageLines{
			Page: i + 1, OriginX: mb[0], OriginY: mb[1],
			Width: mb[2] - mb[0], Height: mb[3] - mb[1], Rotation: page.Rotation(),
		}
		if pl.Rotation == 0 {
			pl.Lines = pageTextLines(page)
		}
		out = append(out, pl)
	}
	return out, nil
}

// textSpanBounds returns the visual bottom and top of a span in PDF user
// space. Y is the baseline, so leave room for descenders below it.
func textSpanBounds(s gopdf.TextSpan) (bottom, top float64) {
	size := spanFontSize(s)
	return s.Y - size*0.25, s.Y + size
}

// spanFontSize estimates the effective font size. gopdf reports the raw Tf
// operand, which can be much larger than the rendered size when the text
// matrix scales it down (for example "/FT8 209 Tf" with "0.05 ... Tm").
// Character advances already include that scale, so derive the size from them
// and keep the declared value only when the two agree.
func spanFontSize(s gopdf.TextSpan) float64 {
	if s.FontSize <= 0 {
		return 1
	}
	units := 0.0
	for _, r := range s.Text {
		switch {
		case unicode.IsSpace(r):
			units += 0.25
		case r < 0x2E80:
			units += 0.5
		default:
			units += 1.0
		}
	}
	if units <= 0 {
		return s.FontSize
	}
	advance := (s.EndX - s.X) / units
	if advance <= 0 {
		return s.FontSize
	}
	if math.Abs(advance-s.FontSize) <= s.FontSize*0.3 {
		return s.FontSize
	}
	return advance
}

func medianSpanSize(spans []gopdf.TextSpan) float64 {
	// CJK advances are one em, so spans containing CJK give the most reliable
	// size estimate. Fall back to all spans for Latin-only lines.
	all := make([]float64, 0, len(spans))
	var cjk []float64
	for _, s := range spans {
		size := spanFontSize(s)
		if size <= 0 {
			continue
		}
		all = append(all, size)
		for _, r := range s.Text {
			if !unicode.IsSpace(r) && r >= 0x2E80 {
				cjk = append(cjk, size)
				break
			}
		}
	}
	sizes := all
	if len(cjk) > 0 {
		sizes = cjk
	}
	if len(sizes) == 0 {
		return 10
	}
	sort.Float64s(sizes)
	return sizes[len(sizes)/2]
}

func pageTextLines(p *gopdf.Page) []TextLine {
	spans, err := p.TextSpans()
	if err != nil || len(spans) == 0 {
		return nil
	}
	groups := make([][]gopdf.TextSpan, 0)
	for _, s := range spans {
		if s.Text == "" {
			continue
		}
		placed := false
		for i := range groups {
			if abs(s.Y-groups[i][0].Y) < 8 {
				groups[i] = append(groups[i], s)
				placed = true
				break
			}
		}
		if !placed {
			groups = append(groups, []gopdf.TextSpan{s})
		}
	}
	lines := make([]TextLine, 0, len(groups))
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool { return group[i].X < group[j].X })
		line := TextLine{X0: group[0].X, X1: group[0].EndX}
		// One height for the whole line: per-span estimates differ between
		// digits and CJK and would otherwise make some boxes taller.
		size := medianSpanSize(group)
		line.Y0, line.Y1 = math.Inf(1), math.Inf(-1)
		prevEndX := 0.0
		prevSize := 0.0
		for _, s := range group {
			if s.X < line.X0 {
				line.X0 = s.X
			}
			if s.EndX > line.X1 {
				line.X1 = s.EndX
			}
			// Use the line's median size for every span. Per-span estimates
			// are noisy for wide Latin letters (an "M" can look twice as
			// large) and would make a single box taller than its line.
			if bottom := s.Y - size*0.25; bottom < line.Y0 {
				line.Y0 = bottom
			}
			if top := s.Y + size; top > line.Y1 {
				line.Y1 = top
			}
			if line.Text != "" {
				gap := s.X - prevEndX
				if prevSize > 0 && gap > prevSize*0.3 {
					line.Text += " "
				}
			}
			segStart := len(line.Text)
			line.Text += s.Text
			line.Segments = append(line.Segments, TextSegment{
				Text: s.Text, X0: s.X, Y0: s.Y - size*0.25, X1: s.EndX, Y1: s.Y + size,
				Start: segStart, End: len(line.Text),
			})
			prevEndX = s.EndX
			prevSize = s.FontSize
		}
		lines = append(lines, line)
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Y0 > lines[j].Y0 })
	return lines
}
