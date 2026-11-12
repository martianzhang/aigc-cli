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
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case bytes.HasPrefix(b, []byte("%PDF-")):
		return FormatPDF, nil
	case bytes.HasPrefix(b, []byte{0x89, 'P', 'N', 'G'}),
		bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}),
		bytes.HasPrefix(b, []byte("RIFF")) && len(b) >= 12 && string(b[8:12]) == "WEBP",
		bytes.HasPrefix(b, []byte("GIF8")),
		bytes.HasPrefix(b, []byte("BM")),
		bytes.HasPrefix(b, []byte{'I', 'I', 0x2A, 0x00}),
		bytes.HasPrefix(b, []byte{'M', 'M', 0x00, 0x2A}):
		return FormatImage, nil
	case ext == ".docx" || ext == ".docm" || ext == ".dotx" || ext == ".dotm":
		return FormatDOCX, nil
	case ext == ".pdf":
		return FormatPDF, nil
	case ext == ".doc":
		return FormatUnknown, fmt.Errorf("legacy .doc is not supported; convert it to .docx first")
	case ext == ".odt" || ext == ".rtf" || ext == ".wps":
		return FormatUnknown, fmt.Errorf("%s is not supported; convert it to .docx or .pdf first", ext)
	case ext == ".ppt" || ext == ".pptx" || ext == ".xls" || ext == ".xlsx":
		return FormatUnknown, fmt.Errorf("%s is not supported by redact", ext)
	}
	return FormatUnknown, fmt.Errorf("unsupported document format: %s", filepath.Base(path))
}

// IsDocumentPath reports whether path names a document container even when
// Detect rejects it (legacy .doc, spreadsheets, slides, ...). Callers use it to
// avoid treating an unsupported binary file as plain text.
func IsDocumentPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".doc", ".docx", ".docm", ".dotx", ".dotm", ".pdf",
		".odt", ".rtf", ".wps", ".ppt", ".pptx", ".xls", ".xlsx",
		".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".tif", ".tiff",
		".avif", ".jxl", ".heic", ".heif":
		return true
	}
	return false
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
	defer func() { _ = zr.Close() }()
	text, _ := combineDOCXText(parts)
	return text, nil
}
