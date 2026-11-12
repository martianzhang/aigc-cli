// Package service provides shared business logic used by CLI commands.
// It extracts reusable operations from cmd/ to reduce file size and
// enable testing without cobra dependencies.
package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/fsutil"
	"github.com/martianzhang/aigc-cli/internal/terminal"
)

const (
	// fetchTimeout bounds one resource fetch. Downloads that make progress are
	// still bounded by this overall deadline to avoid infinite hangs.
	fetchTimeout = 5 * time.Minute
	// maxFetchBytes caps a resource held in memory (FetchBytes / DownloadImages).
	maxFetchBytes = 256 << 20
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
	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Try decoding as image first
	raw, err := decodeBase64Image(b64)
	if err == nil {
		ext := imageExtFromMagic(raw)
		if ext == "" {
			ext = ".png"
		}
		filename := filepath.Join(outputDir, fmt.Sprintf("%s_%d%s", prefix, index, ext))
		if err := fsutil.WriteFileAtomic(filename, raw, 0644); err != nil {
			return "", fmt.Errorf("failed to save image: %w", err)
		}
		return filename, nil
	}

	// Decode failed — save as pure base64 text (no headers, base64 -d reads directly)
	filename := filepath.Join(outputDir, fmt.Sprintf("%s_%d.txt", prefix, index))
	if err := fsutil.WriteFileAtomic(filename, []byte(b64), 0644); err != nil {
		return "", fmt.Errorf("failed to save base64 text: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  Saved: %s (raw base64 — could not decode as image)\n", filename)
	fmt.Fprintf(os.Stderr, "  Convert with: base64 -d %s > %s.png\n", filepath.Base(filename), strings.TrimSuffix(filename, ".txt"))
	return filename, nil
}

// decodeBase64Image tries to decode a base64 string as a recognized image.
// Accepts PNG, JPEG, GIF, and WebP (same set as isLikelyImage).
func decodeBase64Image(b64 string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("data too short for image")
	}
	if isLikelyImage(data) {
		return data, nil
	}
	return nil, fmt.Errorf("unknown image format: first bytes %x", data[:4])
}

// imageExtFromMagic returns the file extension for a recognized image format
// based on magic bytes. Returns "" for unknown formats.
func imageExtFromMagic(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	switch {
	case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
		return ".png"
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return ".jpg"
	case data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38:
		return ".gif"
	case data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		len(data) > 12 && data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50:
		return ".webp"
	}
	return ""
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
		_, after, ok := strings.Cut(cleaned, ",")
		if ok {
			b64 := after
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
	if isHTTPURL(cleaned) {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		resp, err := doHTTPGet(ctx, cleaned)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxFetchBytes {
			return nil, fmt.Errorf("response exceeds %d MB", maxFetchBytes>>20)
		}
		return data, nil
	}

	return nil, fmt.Errorf("unsupported image source: %s", cleaned[:min(len(cleaned), 60)])
}

func isHTTPURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

// doHTTPGet performs a GET with a context and rejects non-200 responses.
func doHTTPGet(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
	}
	return resp, nil
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
	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create output directory: %v\n", err)
		return ""
	}

	filename := filepath.Join(outputDir, fmt.Sprintf("%s_%d.txt", prefix, index))
	// Save pure base64 data — no comments, base64 -d reads this directly
	if err := fsutil.WriteFileAtomic(filename, []byte(raw), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save raw data %s: %v\n", filename, err)
		return ""
	}
	fmt.Fprintf(os.Stderr, "  Saved: %s (raw base64 — could not decode as image)\n", filename)
	fmt.Fprintf(os.Stderr, "  Convert with: base64 -d %s > %s.png\n", filepath.Base(filename), strings.TrimSuffix(filename, ".txt"))
	return filename
}

// SavePrompt writes the generation prompt alongside result files as
// <kind>_<taskID>.md (kind is "image" or "video").
func SavePrompt(outputDir, kind, taskID, prompt string) error {
	if prompt == "" {
		return nil
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	filename := filepath.Join(outputDir, fmt.Sprintf("%s_%s.md", kind, taskID))
	content := fmt.Sprintf("# %s\n\n%s\n", taskID, prompt)
	if err := fsutil.WriteFileAtomic(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("save prompt file: %w", err)
	}
	fmt.Printf("Prompt saved: %s\n", filename)
	return nil
}

// downloadFailure turns an all-or-nothing download result into an error so the
// command exits non-zero instead of printing warnings and reporting success.
func downloadFailure(kind string, attempted, saved int, firstErr error) error {
	if attempted == 0 || saved > 0 {
		return nil
	}
	if firstErr != nil {
		return fmt.Errorf("no %s saved: %w", kind, firstErr)
	}
	return fmt.Errorf("no %s saved", kind)
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

// progressThreshold is the response size above which a download is mirrored into
// the terminal title/tab indicator. Small API assets (thumbnails, JSON) would
// only cause title flicker.
const progressThreshold int64 = 1 << 20

// progressReader reports read progress to prog while a body streams to disk.
type progressReader struct {
	r     io.Reader
	total int64
	done  int64
	prog  *terminal.Progress
	last  int
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if pct := int(min(p.done, p.total) * 100 / p.total); pct != p.last {
		p.last = pct
		p.prog.Percent(pct)
	}
	return n, err
}

// saveHTTPResponse writes the HTTP response body to dest atomically.
func saveHTTPResponse(resp *http.Response, dest string) error {
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Peek before writing so an empty body never replaces an existing file.
	body := bufio.NewReader(resp.Body)
	if _, err := body.Peek(1); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("downloaded file is empty")
		}
		return fmt.Errorf("download failed: %w", err)
	}

	var src io.Reader = body
	if resp.ContentLength >= progressThreshold {
		prog := terminal.Default()
		prog.Stage("download")
		defer prog.Finish("download")
		src = &progressReader{r: body, total: resp.ContentLength, prog: prog, last: -1}
	}

	if _, err := fsutil.WriteFileAtomicFromReader(dest, src, 0644); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	return nil
}

// SaveResource saves content from source (HTTP URL, data URI, or base64) to dest.
// For HTTP URLs uses http.DefaultClient with atomic write.
func SaveResource(source, dest string) error {
	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	cleaned := strings.TrimSpace(source)

	if strings.HasPrefix(cleaned, "http://") || strings.HasPrefix(cleaned, "https://") {
		resp, err := http.DefaultClient.Get(cleaned)
		if err != nil {
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()
		return saveHTTPResponse(resp, dest)
	}

	// data URI — strip header and decode
	if strings.HasPrefix(cleaned, "data:") {
		_, after, ok := strings.Cut(cleaned, ",")
		if ok {
			if data, err := decodeBase64Any(after); err == nil {
				return fsutil.WriteFileAtomic(dest, data, 0644)
			}
		}
		return fmt.Errorf("data URI contains no decodable image data")
	}

	// Raw base64 — try to decode
	if data, err := decodeBase64Any(cleaned); err == nil {
		return fsutil.WriteFileAtomic(dest, data, 0644)
	}

	return fmt.Errorf("unsupported source: %.60s", cleaned)
}

// SaveResourceWithBearer saves an HTTP resource with an optional Bearer token.
// If token is empty, falls back to SaveResource.
// Includes a friendlier 401 error message for HuggingFace gated models.
func SaveResourceWithBearer(source, token, dest string) error {
	cleaned := strings.TrimSpace(source)
	if token == "" {
		return SaveResource(source, dest)
	}

	if !strings.HasPrefix(cleaned, "http://") && !strings.HasPrefix(cleaned, "https://") {
		return SaveResource(source, dest)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	req, err := http.NewRequest("GET", cleaned, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Fprintln(os.Stderr, "This model requires authentication. Use --hf-token:")
		fmt.Fprintln(os.Stderr, "  1. Accept terms on the model's HuggingFace page")
		fmt.Fprintln(os.Stderr, "  2. Create a token at https://huggingface.co/settings/tokens")
		fmt.Fprintln(os.Stderr, "  3. aigc-cli X init --hf-token hf_...")
		return fmt.Errorf("HTTP 401 unauthorized")
	}

	return saveHTTPResponse(resp, dest)
}

// SaveResourceWithAuth saves an HTTP resource with an Authorization header.
// Legacy wrapper, prefers SaveResourceWithBearer for new code.
func SaveResourceWithAuth(source, apiKey, dest string) error {
	cleaned := strings.TrimSpace(source)
	if strings.HasPrefix(cleaned, "https://openrouter.ai/api/") {
		return SaveResourceWithBearer(source, apiKey, dest)
	}
	return SaveResource(source, dest)
}

// DownloadFile saves a resource (HTTP URL, data URI, or base64) to outputDir
// with auto-naming: <taskID><ext>. The extension comes from the source URL,
// or is sniffed from the actual content when the URL has no extension.
// A URL without an extension is streamed exactly once: the first bytes are
// buffered for sniffing and the rest goes straight to the temporary file.
func DownloadFile(source, outputDir, taskID string) (string, error) {
	cleaned := strings.TrimSpace(source)
	if ext := extFromSource(cleaned); ext != "" {
		filename := filepath.Join(outputDir, taskID+ext)
		return filename, SaveResource(cleaned, filename)
	}
	if isHTTPURL(cleaned) {
		return downloadHTTPAndSniff(cleaned, outputDir, taskID)
	}

	// data URI / raw base64: decode once, then sniff.
	data, err := FetchBytes(cleaned)
	if err != nil {
		return "", err
	}
	ext := sniffMediaExt(data)
	if ext == "" {
		ext = ".bin"
	}
	filename := filepath.Join(outputDir, taskID+ext)
	return filename, fsutil.WriteFileAtomic(filename, data, 0644)
}

// downloadHTTPAndSniff streams one HTTP response to a temp file, sniffs the
// leading bytes for the real extension, then renames it into place.
func downloadHTTPAndSniff(source, outputDir, taskID string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	resp, err := doHTTPGet(ctx, source)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp(outputDir, "."+taskID+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	head := make([]byte, 512)
	n, readErr := io.ReadFull(resp.Body, head)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		cleanup()
		return "", fmt.Errorf("read %s: %w", source, readErr)
	}
	if n > 0 {
		if _, err := tmp.Write(head[:n]); err != nil {
			cleanup()
			return "", fmt.Errorf("write temp file: %w", err)
		}
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		cleanup()
		return "", fmt.Errorf("download %s: %w", source, err)
	}

	ext := sniffMediaExt(head[:n])
	if ext == "" {
		ext = extFromContentType(resp.Header.Get("Content-Type"))
	}
	if ext == "" {
		ext = ".bin"
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("close temp file: %w", err)
	}

	final := filepath.Join(outputDir, taskID+ext)
	if err := os.Rename(tmpName, final); err != nil {
		// Windows does not replace an existing target.
		_ = os.Remove(final)
		if retryErr := os.Rename(tmpName, final); retryErr != nil {
			_ = os.Remove(tmpName)
			return "", fmt.Errorf("save %s: %w", final, retryErr)
		}
	}
	return final, nil
}

// sniffMediaExt recognizes image, video and audio containers by magic bytes.
func sniffMediaExt(data []byte) string {
	if ext := imageExtFromMagic(data); ext != "" {
		return ext
	}
	if len(data) >= 12 {
		switch {
		case string(data[4:8]) == "ftyp":
			switch string(data[8:12]) {
			case "M4A ", "M4B ", "M4P ":
				return ".m4a"
			default:
				return ".mp4"
			}
		case string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE":
			return ".wav"
		}
	}
	if len(data) >= 4 {
		switch {
		case data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3:
			return ".webm"
		case string(data[0:4]) == "OggS":
			return ".ogg"
		case string(data[0:4]) == "fLaC":
			return ".flac"
		}
	}
	if len(data) >= 3 && string(data[0:3]) == "ID3" {
		return ".mp3"
	}
	if len(data) >= 2 && data[0] == 0xFF && (data[1] == 0xFB || data[1] == 0xF3 || data[1] == 0xF2) {
		return ".mp3"
	}
	return ""
}

// extFromContentType maps the response content type to a file extension when
// magic-byte sniffing is inconclusive.
func extFromContentType(contentType string) string {
	mediaType, _, _ := strings.Cut(contentType, ";")
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "audio/flac":
		return ".flac"
	case "audio/mp4", "audio/m4a":
		return ".m4a"
	}
	return ""
}
