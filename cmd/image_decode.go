package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// runLocalDecode implements the pure local decode/convert mode (no API call).
// Decodes base64 text files (data URI / raw base64) to real images, and/or
// converts image format via --output-format, saving results to the output dir.
func runLocalDecode(imageURLs []string, targetFormat string) error {
	if len(imageURLs) == 0 {
		return fmt.Errorf("--image-url is required for local decode mode")
	}

	outDir := shared.OutputDir
	if outDir == "" {
		outDir = "."
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	var results []string
	for _, src := range imageURLs {
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			fmt.Fprintf(os.Stderr, "Warning: skipping remote URL (local files only): %s\n", src)
			continue
		}
		if !isFile(src) && !strings.HasPrefix(src, "data:") {
			fmt.Fprintf(os.Stderr, "Warning: file not found: %s\n", src)
			continue
		}

		dest, created, err := resolveImageSource(src, targetFormat, outDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: decode %s: %v\n", src, err)
			continue
		}
		if !created {
			fmt.Fprintf(os.Stderr, "Warning: %s: no conversion needed\n", src)
			continue
		}
		results = append(results, dest)
	}

	if len(results) == 0 {
		return fmt.Errorf("no files were decoded")
	}
	fmt.Println("Decode results:")
	for _, r := range results {
		fmt.Printf("  %s\n", r)
		if genPreview {
			if e := service.PreviewFile(r); e != nil {
				fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
			}
		}
	}
	return nil
}

// resolveImageSource converts one --image-url entry into a real image file in
// destDir. Returns the destination path and whether a new file was created.
//
//   - Remote URLs and unknown strings pass through unchanged (created=false).
//   - data: URIs and base64 text files are decoded to real images.
//   - Real image files are rewritten only when targetFormat requests a format
//     change; otherwise they pass through unchanged.
func resolveImageSource(src, targetFormat, destDir string) (string, bool, error) {
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
	case isFile(s):
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
	if targetFormat == "" && imgcodec.SniffImageExt(content) != "" && isFile(s) {
		return s, false, nil
	}

	decoded, ext, err := service.DecodeImageInput(content, targetFormat)
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", false, fmt.Errorf("create directory %s: %w", destDir, err)
	}
	dest := filepath.Join(destDir, base+ext)
	if isFile(s) && filepath.Clean(dest) == filepath.Clean(s) {
		return "", false, fmt.Errorf("output %s would overwrite the source file; use a different --output-format or output dir", dest)
	}
	if err := os.WriteFile(dest, decoded, 0644); err != nil {
		return "", false, fmt.Errorf("write %s: %w", dest, err)
	}
	return dest, true, nil
}

// decodeImageURLsInline preprocesses --image-url entries in a provider-agnostic
// way: base64 text files and data URIs are converted to inline data URIs that
// OpenAI-compatible APIs accept natively. Real image files and remote URLs pass
// through unchanged (they use the existing upload path).
func decodeImageURLsInline(imageURLs []string, targetFormat string) ([]string, error) {
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
	case isFile(s):
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

	decoded, ext, err := service.DecodeImageInput(content, targetFormat)
	if err != nil {
		return "", false, err
	}
	mime := service.ImageExtMIME(ext)
	if mime == "" {
		mime = "image"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(decoded), true, nil
}
