package webfetch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// DefaultMediaMaxBytes bounds a single media download (2 GiB).
const DefaultMediaMaxBytes int64 = 2 << 30

// normalizeMediaType strips parameters and lowercases a Content-Type value.
func normalizeMediaType(contentType string) string {
	return strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
}

// extensionForMediaType maps common media types to a file extension.
func extensionForMediaType(mediaType string) string {
	switch mediaType {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/avif":
		return ".avif"
	case "image/svg+xml":
		return ".svg"
	case "video/mp4", "application/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "video/x-matroska":
		return ".mkv"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/ogg", "application/ogg":
		return ".ogg"
	case "audio/flac":
		return ".flac"
	case "audio/mp4":
		return ".m4a"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	default:
		return ""
	}
}

// mediaFileName resolves Options.MediaName or the URL basename into a safe file
// name. An explicit name ending with an extension is used verbatim.
func mediaFileName(opts Options, rawURL, mediaType string) (string, error) {
	name := strings.TrimSpace(opts.MediaName)
	if name != "" {
		if err := ValidateMediaName(name); err != nil {
			return "", err
		}
		if filepath.Ext(name) == "" {
			name += mediaExtension(rawURL, mediaType)
		}
		return name, nil
	}

	if u, err := url.Parse(rawURL); err == nil {
		name = path.Base(u.Path)
		if name == "." || name == "/" || name == "" {
			name = ""
		}
	}
	if name != "" {
		ext := strings.ToLower(filepath.Ext(name))
		detected := mediaExtension(rawURL, mediaType)
		if detected != "" && ext != detected {
			// Trust the response over a misleading URL suffix (download.php,
			// image.jpg served as video/mp4, …).
			name = strings.TrimSuffix(name, filepath.Ext(name)) + detected
		}
	}
	if name == "" {
		name = "media-" + time.Now().Format("20060102-150405")
		if ext := mediaExtension(rawURL, mediaType); ext != "" {
			name += ext
		}
	}
	if err := ValidateMediaName(name); err != nil {
		return "", err
	}
	return name, nil
}

// mediaExtension returns the extension implied by the content type, then the
// URL path, then .bin.
func mediaExtension(rawURL, mediaType string) string {
	if ext := extensionForMediaType(mediaType); ext != "" {
		return ext
	}
	if u, err := url.Parse(rawURL); err == nil {
		ext := strings.ToLower(filepath.Ext(u.Path))
		if isKnownMediaExt(ext) {
			return ext
		}
	}
	return ".bin"
}

func isKnownMediaExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif", ".svg",
		".mp4", ".mov", ".webm", ".mkv",
		".mp3", ".wav", ".ogg", ".flac", ".m4a", ".aac",
		".pdf", ".zip", ".glb", ".gltf", ".fbx", ".obj", ".blend":
		return true
	default:
		return false
	}
}

// ValidateMediaName rejects path traversal and control characters. --name is a
// file name, not a path; -o selects the directory.
func ValidateMediaName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid media name %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("media name %q must not contain a path separator; use -o for the directory", name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("media name %q contains a control character", name)
		}
	}
	if len(name) > 255 {
		return fmt.Errorf("media name is too long (%d bytes)", len(name))
	}
	return nil
}

// saveMedia streams a binary response to MediaDir and returns the saved-file
// metadata. It writes through .part + rename so an interrupted download never
// looks like a complete file.
func saveMedia(r io.Reader, opts Options, rawURL, finalURL, mediaType, contentType string, total int64) (*Document, error) {
	name, err := mediaFileName(opts, rawURL, mediaType)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(opts.MediaDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output directory %s: %w", opts.MediaDir, err)
	}
	target := filepath.Join(opts.MediaDir, name)
	if !opts.Force {
		if _, err := os.Stat(target); err == nil {
			return nil, fmt.Errorf("file exists: %s (use --force to overwrite)", target)
		}
	}

	tmp, err := os.CreateTemp(opts.MediaDir, ".aigc-fetch-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		_ = tmp.Close()
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	limit := opts.MediaMaxBytes
	if limit <= 0 {
		limit = DefaultMediaMaxBytes
	}
	if total > 0 && total > limit {
		return nil, fmt.Errorf("media is %d MiB, over the %d MiB limit", total/(1<<20), limit/(1<<20))
	}
	hasher := sha256.New()
	writer := io.MultiWriter(tmp, hasher)
	buf := make([]byte, 32*1024)
	var written int64
	lastReport := time.Now()
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			if _, werr := writer.Write(buf[:n]); werr != nil {
				return nil, fmt.Errorf("write media: %w", werr)
			}
			written += int64(n)
			if written > limit {
				return nil, fmt.Errorf("media exceeds the %d MiB limit", limit/(1<<20))
			}
			if opts.Progress != nil &&
				(time.Since(lastReport) >= 150*time.Millisecond || readErr == io.EOF ||
					(total > 0 && written >= total)) {
				opts.Progress(written, total)
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read media: %w", readErr)
		}
	}
	if opts.Progress != nil {
		opts.Progress(written, total)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close media file: %w", err)
	}
	if opts.Force {
		_ = os.Remove(target) // Windows cannot rename over an existing file.
	}
	if err := os.Rename(tmpName, target); err != nil {
		return nil, fmt.Errorf("save media to %s: %w", target, err)
	}
	success = true

	return &Document{
		URL:         rawURL,
		FinalURL:    finalURL,
		ContentType: firstNonEmpty(contentType, mediaType),
		Bytes:       written,
		Media:       true,
		MediaType:   mediaType,
		SavedPath:   target,
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}
