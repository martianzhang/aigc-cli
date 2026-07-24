// Package pdf provides PDF processing in Go: text extraction for born-digital
// PDFs via razvandimescu/gopdf (pure Go, no CGO), and page-to-image rendering
// via mutool (external CLI, muPDF tools — required only for scanned/image-based
// PDFs). The mutool dependency is optional — text-based PDFs work without it.
package pdf

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	gopdf "github.com/razvandimescu/gopdf/pdf"
)

var gopdfOpen = gopdf.OpenFile

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
func ExtractText(path string) ([]PageText, error) {
	doc, err := gopdfOpen(path)
	if err != nil {
		return nil, err
	}

	numPages := doc.NumPages()
	pages := make([]PageText, 0, numPages)

	for i := 0; i < numPages; i++ {
		text := extractPageText(doc.Page(i))
		pages = append(pages, PageText{Page: i + 1, Text: text})
	}

	return pages, nil
}

// vertLabelInfo captures a detected vertical label with its spatial bounds.
type vertLabelInfo struct {
	text    string
	topY    float64
	bottom  float64
	centerX float64
	spanIdx []int
}

// extractPageText extracts text from a single PDF page using TextSpans().
func extractPageText(p *gopdf.Page) string {
	spans, err := p.TextSpans()
	if err != nil || len(spans) == 0 {
		return ""
	}

	vertLabels := detectVerticalLabels(spans)

	isVerticalLabel := make(map[int]bool)
	for _, vl := range vertLabels {
		for _, idx := range vl.spanIdx {
			if idx >= 0 && idx < len(spans) {
				isVerticalLabel[idx] = true
			}
		}
	}

	type contentSpan struct {
		idx int
		gopdf.TextSpan
	}
	var contentSpans []contentSpan
	for i, s := range spans {
		if !isVerticalLabel[i] && s.Text != "" {
			contentSpans = append(contentSpans, contentSpan{idx: i, TextSpan: s})
		}
	}

	type lineGroup struct {
		y     float64
		spans []contentSpan
	}
	var groups []lineGroup
	for _, cs := range contentSpans {
		found := false
		for j := range groups {
			if abs(cs.Y-groups[j].y) < 8 {
				groups[j].spans = append(groups[j].spans, cs)
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, lineGroup{y: cs.Y, spans: []contentSpan{cs}})
		}
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].y > groups[j].y })

	usedLabel := make(map[int]bool)
	var result []string
	for i := range groups {
		g := &groups[i]
		sort.Slice(g.spans, func(a, b int) bool {
			return g.spans[a].X < g.spans[b].X
		})

		bestLabel := ""
		bestDist := 1e9
		for li, vl := range vertLabels {
			if usedLabel[li] {
				continue
			}
			center := (vl.topY + vl.bottom) / 2
			d := abs(g.y - center)
			if d < 25 && d < bestDist {
				bestDist = d
				bestLabel = vl.text
				usedLabel[li] = true
			}
		}

		spans := make([]gopdf.TextSpan, len(g.spans))
		for si, cs := range g.spans {
			spans[si] = cs.TextSpan
		}
		lineText := joinSpansWithSpacing(spans)

		if bestLabel != "" {
			lineText = bestLabel + " " + lineText
		}

		if lineText != "" {
			result = append(result, lineText)
		}
	}

	return strings.Join(result, "\n")
}

// joinSpansWithSpacing joins spans (already sorted by X) into a single string,
// inserting spaces between them based on the horizontal gap.
func joinSpansWithSpacing(spans []gopdf.TextSpan) string {
	if len(spans) == 0 {
		return ""
	}

	// Collect non-empty, trimmed text with X/EndX for spacing decisions.
	var texts []string
	var xCoords []float64
	var endXs []float64
	var fontSizes []float64

	for _, s := range spans {
		t := strings.TrimSpace(s.Text)
		if t == "" {
			continue
		}
		texts = append(texts, t)
		xCoords = append(xCoords, s.X)
		eX := s.EndX
		if eX <= s.X {
			eX = s.X + float64(len([]rune(s.Text)))*s.FontSize*0.5
		}
		endXs = append(endXs, eX)
		fontSizes = append(fontSizes, s.FontSize)
	}

	if len(texts) == 0 {
		return ""
	}

	var sb strings.Builder
	prevEndX := endXs[0]
	for i, text := range texts {
		if i > 0 {
			gap := xCoords[i] - prevEndX
			threshold := fontSizes[i] * 2
			if threshold < 2 {
				threshold = 2
			}
			if gap > threshold {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(text)
		prevEndX = endXs[i]
	}

	// Add a trailing space so the next line doesn't stick to this one.
	return sb.String() + " "
}

// detectVerticalLabels finds runs of single-CJK spans at similar X
// with consecutive Y steps (5–22px), and returns structured label info.
func detectVerticalLabels(spans []gopdf.TextSpan) []vertLabelInfo {
	type bucketEntry struct {
		y       float64
		text    string
		spanIdx int
	}

	buckets := make(map[int][]bucketEntry)
	for i, s := range spans {
		if !isSingleCJK(s.Text) {
			continue
		}
		bk := int(s.X/20) * 20
		buckets[bk] = append(buckets[bk], bucketEntry{
			y:       s.Y,
			text:    s.Text,
			spanIdx: i,
		})
	}

	var labels []vertLabelInfo
	for _, entries := range buckets {
		if len(entries) < 2 {
			continue
		}

		sort.Slice(entries, func(i, j int) bool {
			return entries[i].y > entries[j].y
		})

		runStart := 0
		for i := 1; i <= len(entries); i++ {
			var step float64
			if i < len(entries) {
				step = abs(entries[i].y - entries[i-1].y)
			}
			endRun := i >= len(entries) || step > 22 || step < 4
			if endRun && i-runStart >= 2 {
				text := ""
				topY := entries[runStart].y
				bottom := entries[i-1].y
				var spanIdx []int
				for k := runStart; k < i; k++ {
					text += entries[k].text
					spanIdx = append(spanIdx, entries[k].spanIdx)
				}
				labels = append(labels, vertLabelInfo{
					text:    text,
					topY:    topY,
					bottom:  bottom,
					centerX: entries[0].y,
					spanIdx: spanIdx,
				})
			}
			if endRun && i < len(entries) {
				runStart = i
			}
		}
	}

	return labels
}

// isSingleCJK reports whether s is exactly one CJK character.
func isSingleCJK(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	r := []rune(s)
	if len(r) != 1 {
		return false
	}
	c := r[0]
	return (c >= 0x4E00 && c <= 0x9FFF) ||
		(c >= 0x3400 && c <= 0x4DBF) ||
		(c >= 0xF900 && c <= 0xFAFF) ||
		(c >= 0xFF01 && c <= 0xFF60)
}

// IsScanned reports whether a PDF is a scanned/image document based on its
// extracted text.
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

// RenderToImages renders every page of a PDF to PNG images using mutool.
func RenderToImages(pdfPath, outputDir string, dpi int) ([]string, error) {
	return renderPages(pdfPath, outputDir, nil, dpi)
}

// SelectedPages renders only the specified page numbers (1-based) from a PDF.
func SelectedPages(pdfPath, outputDir string, pageNums []string, dpi int) ([]string, error) {
	return renderPages(pdfPath, outputDir, pageNums, dpi)
}

func renderPages(pdfPath, outputDir string, pageNums []string, dpi int) ([]string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}

	mutoolPath, err := findMutool()
	if err != nil {
		return nil, err
	}

	outPattern := filepath.Join(outputDir, "page-%d.png")
	args := []string{"draw", "-o", outPattern, "-r", strconv.Itoa(dpi), pdfPath}
	if len(pageNums) > 0 {
		args = append(args, pageNums...)
	}

	cmd := exec.Command(mutoolPath, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, err
	}

	var images []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".png") || strings.HasSuffix(e.Name(), ".PNG")) {
			images = append(images, filepath.Join(outputDir, e.Name()))
		}
	}
	if len(images) == 0 {
		return nil, nil
	}

	sort.Slice(images, func(i, j int) bool {
		return extractPageNum(images[i]) < extractPageNum(images[j])
	})

	return images, nil
}

func extractPageNum(path string) int {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	parts := strings.Split(name, "-")
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return n
}

func configBinDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "aigc-cli", "bin")
}

func findMutool() (string, error) {
	if dir := configBinDir(); dir != "" {
		candidate := filepath.Join(dir, "mutool")
		if s, err := os.Stat(candidate); err == nil && !s.IsDir() {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath("mutool"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("mutool not found — install mupdf-tools: https://mupdf.com/downloads")
}

// MutoolInstallHint returns a help message for installing mutool.
func MutoolInstallHint() string {
	return `this PDF appears to be a scanned document (no extractable text layer).
To OCR it, install mupdf-tools (mutool) to convert PDF pages to images:

  macOS:  brew install mupdf-tools
  Linux:  apt install mupdf-tools    # or pacman -S mupdf-tools
  Windows: https://mupdf.com/downloads

Then run 'aigc-cli ocr scan <file>' again.`
}

func abs(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}
