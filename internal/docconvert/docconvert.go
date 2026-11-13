// Package docconvert extracts readable Markdown from documents that
// `aigc-cli preview` shows in the terminal: PDF text layers, EPUB, DOCX and
// HTML. It is pure Go; scanned PDFs need mutool, which the caller handles.
package docconvert

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoTextLayer reports a PDF without an extractable text layer (typically a
// scanned document). Callers can fall back to a page renderer or the system
// viewer.
var ErrNoTextLayer = errors.New("PDF has no extractable text layer (scanned document)")

// ErrComplexLayout reports a PDF whose text layer cannot be turned into a
// faithful Markdown preview (multi-column text, rotated pages, mixed scanned
// pages). Callers should prefer a page renderer or the system viewer.
var ErrComplexLayout = errors.New("PDF layout is too complex for reliable text preview")

const (
	// maxDocumentBytes bounds one input file.
	maxDocumentBytes = 256 << 20
	// maxEntryBytes bounds one archive member.
	maxEntryBytes = 64 << 20
)

var extensions = []string{".pdf", ".epub", ".docx", ".html", ".htm"}

// Supported reports whether path has a document extension this package can
// convert. It does not check that the file exists.
func Supported(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, candidate := range extensions {
		if ext == candidate {
			return true
		}
	}
	return false
}

// Markdown converts a supported document to Markdown.
func Markdown(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot access %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}
	if info.Size() > maxDocumentBytes {
		return "", fmt.Errorf("%s exceeds %d MB", filepath.Base(path), maxDocumentBytes>>20)
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return fromPDF(path)
	case ".epub":
		return fromEPUB(path)
	case ".docx":
		return fromDOCX(path)
	case ".html", ".htm":
		return fromHTML(path)
	default:
		return "", fmt.Errorf("unsupported document format %q", filepath.Ext(path))
	}
}

// readZipFile reads one archive member with a size cap.
func readZipFile(file *zip.File, limit int64) ([]byte, error) {
	if int64(file.UncompressedSize64) > limit {
		return nil, fmt.Errorf("%s exceeds %d MB", file.Name, limit>>20)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", file.Name, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file.Name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d MB", file.Name, limit>>20)
	}
	return data, nil
}
