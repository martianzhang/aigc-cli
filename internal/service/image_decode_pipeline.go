package service

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

// ResolveImageSource converts one --image-url entry into a real image file in
// destDir and reports whether a new file was created.
//
//   - Remote URLs and unknown strings pass through unchanged (created=false).
//   - data: URIs and base64 text files are decoded to real images.
//   - Real image files are rewritten only when targetFormat requests a format
//     change; otherwise they pass through unchanged.
func ResolveImageSource(src, targetFormat, destDir string) (string, bool, error) {
	s := strings.TrimSpace(src)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s, false, nil
	}

	var content []byte
	var base string
	switch {
	case strings.HasPrefix(s, "data:"):
		content = []byte(s)
		base = "image"
	case fileExists(s):
		data, err := os.ReadFile(s)
		if err != nil {
			return "", false, fmt.Errorf("read %s: %w", s, err)
		}
		content = data
		base = strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
	default:
		return s, false, nil // non-file, non-URL string: pass through
	}

	// Real image file with no format conversion requested → use as-is.
	if targetFormat == "" && imgcodec.SniffImageExt(content) != "" && fileExists(s) {
		return s, false, nil
	}

	decoded, ext, err := DecodeImageInput(content, targetFormat)
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", false, fmt.Errorf("create directory %s: %w", destDir, err)
	}
	dest := filepath.Join(destDir, base+ext)
	if fileExists(s) && filepath.Clean(dest) == filepath.Clean(s) {
		return "", false, fmt.Errorf("output %s would overwrite the source file; use a different --output-format or output dir", dest)
	}
	if err := os.WriteFile(dest, decoded, 0644); err != nil {
		return "", false, fmt.Errorf("write %s: %w", dest, err)
	}
	return dest, true, nil
}

// DecodeImageURLsInline preprocesses --image-url entries in a provider-agnostic
// way: base64 text files and data URIs are converted to inline data URIs that
// OpenAI-compatible APIs accept natively. Real image files and remote URLs pass
// through unchanged (they use the existing upload path).
func DecodeImageURLsInline(imageURLs []string, targetFormat string) ([]string, error) {
	resolved := make([]string, 0, len(imageURLs))
	for _, src := range imageURLs {
		converted, ok, err := toDataURI(src, targetFormat)
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", src, err)
		}
		if !ok {
			resolved = append(resolved, src)
			continue
		}
		resolved = append(resolved, converted)
	}
	return resolved, nil
}

// toDataURI converts one --image-url entry into an inline data URI when it is
// base64 text or a data URI. Returns (entry, false) when no conversion applies:
// remote URLs and real image files pass through.
func toDataURI(src, targetFormat string) (string, bool, error) {
	s := strings.TrimSpace(src)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s, false, nil
	}

	var content []byte
	switch {
	case strings.HasPrefix(s, "data:"):
		content = []byte(s)
	case fileExists(s):
		data, err := os.ReadFile(s)
		if err != nil {
			return "", false, fmt.Errorf("read %s: %w", s, err)
		}
		// Real image file → keep for the existing upload path.
		if imgcodec.SniffImageExt(data) != "" {
			return s, false, nil
		}
		content = data
	default:
		return s, false, nil // non-file, non-URL string: pass through
	}

	// Text outputs (base64/datauri) only make sense in pure-local mode; in
	// inline mode keep the detected image format.
	switch strings.ToLower(strings.TrimSpace(targetFormat)) {
	case "base64", "datauri", "data-uri":
		targetFormat = ""
	}

	decoded, ext, err := DecodeImageInput(content, targetFormat)
	if err != nil {
		return "", false, err
	}
	mime := ImageExtMIME(ext)
	if mime == "" {
		mime = "image"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(decoded), true, nil
}

// fileExists reports whether path is an existing regular (non-directory) file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
