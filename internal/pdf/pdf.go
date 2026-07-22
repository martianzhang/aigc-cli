// Package pdf provides PDF processing in Go: text extraction for born-digital
// PDFs via ledongthuc/pdf (pure Go), and page-to-image rendering via mutool
// (external CLI, muPDF tools — required only for scanned/image-based PDFs).
// No CGO. The mutool dependency is optional — text-based PDFs work without it.
package pdf

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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

// RenderToImages renders every page of a PDF to PNG images using mutool
// (muPDF command-line tool). The output directory is created if it does not
// exist. Returns the paths to the rendered PNG files sorted by page number.
//
// mutool must be installed separately:
//
//	macOS:  brew install mupdf-tools
//	Linux:  apt install mupdf-tools    # or pacman -S mupdf-tools
//	Windows: https://mupdf.com/downloads
func RenderToImages(pdfPath, outputDir string, dpi int) ([]string, error) {
	return renderPages(pdfPath, outputDir, nil, dpi)
}

// SelectedPages renders only the specified page numbers (1-based) from a PDF
// to PNG images using mutool. pageNums is a list of page specifiers that
// mutool accepts (e.g. "1", "1-3", "1,3,5").
// Returns paths to the rendered PNG files sorted by page number.
func SelectedPages(pdfPath, outputDir string, pageNums []string, dpi int) ([]string, error) {
	return renderPages(pdfPath, outputDir, pageNums, dpi)
}

// renderPages is the shared implementation for RenderToImages and SelectedPages.
// pageNums = nil means all pages.
func renderPages(pdfPath, outputDir string, pageNums []string, dpi int) ([]string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
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
		return nil, fmt.Errorf("mutool draw failed: %w", err)
	}

	// Enumerate rendered PNG files.
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, fmt.Errorf("read output dir: %w", err)
	}

	var images []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".png") || strings.HasSuffix(e.Name(), ".PNG")) {
			images = append(images, filepath.Join(outputDir, e.Name()))
		}
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("mutool produced no output images")
	}

	sort.Slice(images, func(i, j int) bool {
		return extractPageNum(images[i]) < extractPageNum(images[j])
	})

	return images, nil
}

// extractPageNum extracts the page number from a filename like "page-3.png".
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

// configBinDir returns ~/.config/aigc-cli/bin — the directory for bundled
// CLI tools (mutool, etc.). This lets us ship tools alongside the config
// without requiring system-wide install.
func configBinDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "aigc-cli", "bin")
}

// findMutool locates the mutool binary, checking ~/.config/aigc-cli/bin/ first
// then falling back to PATH. Returns the full path or an error with install hint.
func findMutool() (string, error) {
	// Check config bin dir first.
	if dir := configBinDir(); dir != "" {
		candidate := filepath.Join(dir, "mutool")
		if s, err := os.Stat(candidate); err == nil && !s.IsDir() {
			return candidate, nil
		}
		// macOS homebrew installs as mutool (not mupdf-gl/mupdf-mr)
	}
	// Fall back to PATH.
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
