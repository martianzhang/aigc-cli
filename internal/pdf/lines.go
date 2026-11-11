package pdf

import (
	"sort"

	gopdf "github.com/razvandimescu/gopdf/pdf"
)

// TextLine is one reconstructed line with a PDF-point bounding box. Y0/Y1 use
// PDF user-space coordinates (origin bottom-left).
type TextLine struct {
	Text string
	X0   float64
	Y0   float64
	X1   float64
	Y1   float64
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
		line.Y0, line.Y1 = group[0].Y, group[0].Y+group[0].FontSize
		prevEndX := 0.0
		prevSize := 0.0
		for _, s := range group {
			if s.X < line.X0 {
				line.X0 = s.X
			}
			if s.EndX > line.X1 {
				line.X1 = s.EndX
			}
			if s.Y < line.Y0 {
				line.Y0 = s.Y
			}
			if s.Y+s.FontSize > line.Y1 {
				line.Y1 = s.Y + s.FontSize
			}
			if line.Text != "" {
				gap := s.X - prevEndX
				if prevSize > 0 && gap > prevSize*0.3 {
					line.Text += " "
				}
			}
			line.Text += s.Text
			prevEndX = s.EndX
			prevSize = s.FontSize
		}
		lines = append(lines, line)
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].Y0 > lines[j].Y0 })
	return lines
}
