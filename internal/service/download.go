// Package service provides shared business logic used by CLI commands.
// It extracts reusable operations from cmd/ to reduce file size and
// enable testing without cobra dependencies.
package service

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ExtractExt returns the file extension from a URL, defaulting to ".mp4".
func ExtractExt(rawURL string) string {
	if idx := strings.Index(rawURL, "?"); idx != -1 {
		rawURL = rawURL[:idx]
	}
	ext := filepath.Ext(rawURL)
	if ext == "" {
		return ".mp4"
	}
	return ext
}

// SaveBase64Image tries to decode and save a base64-encoded image.
// On success, saves as an image file. On failure, saves the raw base64 data
// as a text file with instructions for manual conversion.
func SaveBase64Image(outputDir, prefix, b64 string, index int) (string, error) {
	// Try decoding as image first
	raw, err := decodeBase64Image(b64)
	if err == nil {
		ext := ".png"
		filename := filepath.Join(outputDir, fmt.Sprintf("%s_%d%s", prefix, index, ext))
		if err := os.WriteFile(filename, raw, 0644); err != nil {
			return "", fmt.Errorf("failed to save image: %w", err)
		}
		return filename, nil
	}

	// Decode failed — save as pure base64 text (no headers, base64 -d reads directly)
	filename := filepath.Join(outputDir, fmt.Sprintf("%s_%d.txt", prefix, index))
	if err := os.WriteFile(filename, []byte(b64), 0644); err != nil {
		return "", fmt.Errorf("failed to save base64 text: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  Saved: %s (raw base64 — could not decode as image)\n", filename)
	fmt.Fprintf(os.Stderr, "  Convert with: base64 -d %s > %s.png\n", filepath.Base(filename), strings.TrimSuffix(filename, ".txt"))
	return filename, nil
}

// decodeBase64Image tries to decode a base64 string as a PNG/JPEG image.
func decodeBase64Image(b64 string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	// Verify it looks like an image (PNG magic bytes: 89 50 4E 47 or JPEG: FF D8 FF)
	if len(data) < 4 {
		return nil, fmt.Errorf("data too short for image")
	}
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return data, nil // PNG
	}
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return data, nil // JPEG
	}
	return nil, fmt.Errorf("unknown image format: first bytes %x", data[:4])
}

// FetchBytes gets raw bytes from a URL, data URI, or base64 string.
// Supports:
//   - HTTP/HTTPS URLs (download via GET with http.DefaultClient)
//   - data: URIs (e.g. data:image/png;base64,...)
//   - Raw base64 strings
func FetchBytes(rawURL string) ([]byte, error) {
	// Strip whitespace that may appear in API responses
	cleaned := strings.TrimSpace(rawURL)

	// data: URI
	if strings.HasPrefix(cleaned, "data:") {
		commaIdx := strings.Index(cleaned, ",")
		if commaIdx >= 0 {
			b64 := cleaned[commaIdx+1:]
			if data, err := decodeBase64Any(b64); err == nil {
				return data, nil
			}
		}
		// Malformed data URI (no comma or undecodable) — no data to salvage
		return nil, fmt.Errorf("data URI contains no decodable image data")
	}

	// Raw base64 (JPEG starts with /9j, PNG with iVBOR, or just long base64)
	if len(cleaned) > 20 {
		if data, err := decodeBase64Any(cleaned); err == nil && isLikelyImage(data) {
			return data, nil
		}
	}

	// Regular HTTP GET (absolute URL only)
	if strings.HasPrefix(cleaned, "http://") || strings.HasPrefix(cleaned, "https://") {
		resp, err := http.Get(cleaned)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}

	return nil, fmt.Errorf("unsupported image source: %s", cleaned[:min(len(cleaned), 60)])
}

// decodeBase64Any tries multiple base64 decoding strategies.
func decodeBase64Any(s string) ([]byte, error) {
	// Remove any whitespace (common in API responses)
	s = stripWhitespace(s)

	// Try standard (padded)
	if data, err := base64.StdEncoding.DecodeString(s); err == nil {
		return data, nil
	}
	// Try raw standard (no padding)
	if data, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return data, nil
	}
	// Auto-fix padding and retry
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.StdEncoding.DecodeString(s)
}

// isLikelyImage checks if decoded bytes look like a common image format.
func isLikelyImage(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// PNG: 89 50 4E 47
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return true
	}
	// JPEG: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true
	}
	// GIF: 47 49 46 38
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return true
	}
	// WebP: 52 49 46 46 (RIFF) + ... + 57 45 42 50 (WEBP)
	if data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 && len(data) > 12 {
		if data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
			return true
		}
	}
	return false
}

// stripWhitespace removes all whitespace characters from a string.
func stripWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SaveBase64Fallback saves raw image data as a .txt file (pure base64, no headers).
// Used when FetchBytes cannot decode the data as an image.
func SaveBase64Fallback(outputDir, prefix, raw string, index int) string {
	filename := filepath.Join(outputDir, fmt.Sprintf("%s_%d.txt", prefix, index))
	// Save pure base64 data — no comments, base64 -d reads this directly
	if err := os.WriteFile(filename, []byte(raw), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save raw data %s: %v\n", filename, err)
		return ""
	}
	fmt.Fprintf(os.Stderr, "  Saved: %s (raw base64 — could not decode as image)\n", filename)
	fmt.Fprintf(os.Stderr, "  Convert with: base64 -d %s > %s.png\n", filepath.Base(filename), strings.TrimSuffix(filename, ".txt"))
	return filename
}

// SavePrompt writes the generation prompt alongside result files.
func SavePrompt(outputDir, taskID, prompt string) {
	if prompt == "" {
		return
	}
	filename := filepath.Join(outputDir, fmt.Sprintf("image_%s.md", taskID))
	content := fmt.Sprintf("# %s\n\n%s\n", taskID, prompt)
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save prompt file: %v\n", err)
		return
	}
	fmt.Printf("Prompt saved: %s\n", filename)
}

// extFromSource extracts the file extension from a source URL, stripping query params.
func extFromSource(source string) string {
	if idx := strings.Index(source, "?"); idx >= 0 {
		source = source[:idx]
	}
	ext := filepath.Ext(source)
	if ext == "" || len(ext) > 5 {
		return ""
	}
	return ext
}

// SaveResource saves content from source (HTTP URL, data URI, or base64) to dest.
// For HTTP URLs uses http.DefaultClient with atomic write.
func SaveResource(source, dest string) error {
	cleaned := strings.TrimSpace(source)

	if strings.HasPrefix(cleaned, "http://") || strings.HasPrefix(cleaned, "https://") {
		resp, err := http.DefaultClient.Get(cleaned)
		if err != nil {
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		tmpDest := dest + ".tmp"
		f, err := os.Create(tmpDest)
		if err != nil {
			return fmt.Errorf("cannot create %s: %w", tmpDest, err)
		}

		written, err := io.Copy(f, resp.Body)
		if err != nil {
			f.Close()
			os.Remove(tmpDest)
			return fmt.Errorf("download failed: %w", err)
		}
		f.Close()

		if written == 0 {
			os.Remove(tmpDest)
			return fmt.Errorf("downloaded file is empty")
		}

		if err := os.Rename(tmpDest, dest); err != nil {
			return fmt.Errorf("rename failed: %w", err)
		}
		return nil
	}

	// data URI — strip header and decode
	if strings.HasPrefix(cleaned, "data:") {
		commaIdx := strings.Index(cleaned, ",")
		if commaIdx >= 0 {
			if data, err := decodeBase64Any(cleaned[commaIdx+1:]); err == nil {
				return os.WriteFile(dest, data, 0644)
			}
		}
		return fmt.Errorf("data URI contains no decodable image data")
	}

	// Raw base64 — try to decode
	if data, err := decodeBase64Any(cleaned); err == nil {
		return os.WriteFile(dest, data, 0644)
	}

	return fmt.Errorf("unsupported source: %.60s", cleaned)
}

// DownloadFile saves a resource (HTTP URL, data URI, or base64) to outputDir
// with auto-naming: <taskID><ext>. The extension comes from the source URL.
func DownloadFile(source, outputDir, taskID string) (string, error) {
	ext := extFromSource(source)
	filename := filepath.Join(outputDir, taskID+ext)
	return filename, SaveResource(source, filename)
}
