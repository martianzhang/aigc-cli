// Package docredact adapts redact's text detection to DOCX, PDF and images.
package docredact

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/ocr"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

// Format is the detected input format.
type Format int

const (
	FormatUnknown Format = iota
	FormatDOCX
	FormatPDF
	FormatImage
)

func (f Format) String() string {
	switch f {
	case FormatDOCX:
		return "docx"
	case FormatPDF:
		return "pdf"
	case FormatImage:
		return "image"
	default:
		return "unknown"
	}
}

// OCREngine is the subset of ocr.Engine used by this package.
type OCREngine interface {
	Scan(img image.Image) (*ocr.OCRResult, error)
}

// Options controls document redaction. Style is only used by DOCX; PDF and
// images always use black boxes. DPI applies to PDF rendering.
type Options struct {
	Style       string
	Placeholder string
	Masks       *redactlib.MaskResolver
	Aliases     map[string][]string
	SkipDate    bool
	SkipAmount  bool
	DPI         int
	ModelsDir   string
	OCR         OCREngine
}

// Result is the output of a scan/apply operation.
type Result struct {
	OutputPath string
	Text       string
	Spans      []redactlib.Span
	Review     []redactlib.Span
	Mapping    *redactlib.Mapping
}

// Detect identifies a file by magic bytes, with extension as a fallback.
func Detect(path string) (Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return FormatUnknown, err
	}
	defer f.Close()
	var head [512]byte
	n, _ := f.Read(head[:])
	b := head[:n]
	switch {
	case bytes.HasPrefix(b, []byte("%PDF-")):
		return FormatPDF, nil
	case bytes.HasPrefix(b, []byte{0x89, 'P', 'N', 'G'}),
		bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}),
		bytes.HasPrefix(b, []byte("RIFF")) && len(b) >= 12 && string(b[8:12]) == "WEBP",
		bytes.HasPrefix(b, []byte("GIF8")):
		return FormatImage, nil
	case strings.EqualFold(filepath.Ext(path), ".docx"):
		return FormatDOCX, nil
	case strings.EqualFold(filepath.Ext(path), ".pdf"):
		return FormatPDF, nil
	}
	return FormatUnknown, fmt.Errorf("unsupported document format: %s", filepath.Base(path))
}

// Apply redacts path into outPath. The caller supplies the configured pipeline.
func Apply(ctx context.Context, input, outPath string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	format, err := Detect(input)
	if err != nil {
		return Result{}, err
	}
	if opts.DPI <= 0 {
		opts.DPI = 300
	}
	switch format {
	case FormatImage:
		return applyImage(ctx, input, outPath, pipeline, opts)
	case FormatPDF:
		return applyPDF(ctx, input, outPath, pipeline, opts)
	case FormatDOCX:
		return applyDOCX(ctx, input, outPath, pipeline, opts)
	default:
		return Result{}, fmt.Errorf("unsupported document format: %s", filepath.Base(input))
	}
}

// Scan extracts and detects text without writing an output file.
func Scan(ctx context.Context, input string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	format, err := Detect(input)
	if err != nil {
		return Result{}, err
	}
	if opts.DPI <= 0 {
		opts.DPI = 300
	}
	switch format {
	case FormatImage:
		return scanImage(ctx, input, pipeline, opts)
	case FormatPDF:
		return scanPDF(ctx, input, pipeline, opts)
	case FormatDOCX:
		return scanDOCX(ctx, input, pipeline, opts)
	default:
		return Result{}, fmt.Errorf("unsupported document format: %s", filepath.Base(input))
	}
}

// ExtractText returns plain text for formats that can be inspected without
// OCR (currently DOCX). PDF/image text extraction requires rendering/OCR and is
// handled by Scan.
func ExtractText(input string) (string, error) {
	format, err := Detect(input)
	if err != nil {
		return "", err
	}
	if format != FormatDOCX {
		return "", fmt.Errorf("text extraction is not supported for %s", format)
	}
	parts, zr, err := extractDOCX(input)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	text, _ := combineDOCXText(parts)
	return text, nil
}
