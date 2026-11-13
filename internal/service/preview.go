package service

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/aigc-cli/internal/audio"
	"github.com/martianzhang/aigc-cli/internal/fsutil"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// PreviewFile opens a file or URL with the system default application.
// For image files, it also attempts inline terminal display when stdout is a
// real terminal and the terminal supports it (Kitty, iTerm2, or Sixel).
// URLs are downloaded to a temporary file first.
func PreviewFile(path string) error {
	// Detect URL — download to temp file first
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		data, err := FetchBytes(path)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", path, err)
		}
		// Detect image extension from magic bytes
		ext := detectImageExt(data)
		// Generate a filename from the URL
		saveName := urlToFilename(path, ext)
		if err := fsutil.WriteFileAtomic(saveName, data, 0644); err != nil {
			return fmt.Errorf("failed to save %s: %w", saveName, err)
		}
		ui.Printf("Saved: %s\n", saveName)
		path = saveName
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}

	// Try inline terminal image display for supported terminals. It writes the
	// image bytes as an escape sequence, so it is only safe on a real terminal
	// and when the user has not opted out; everywhere else (pipes, redirected
	// logs, IDE/TUI capture) the system viewer is the only sane fallback.
	if isImageFile(path) && inlinePreviewAllowed() {
		if trySixelImage(path) {
			return nil
		}
		if tryInlineImage(path) {
			return nil
		}
	}

	// Try in-process audio playback (no external app needed), falling back to
	// the system player when this build has no audio backend or no device.
	if isAudioFile(path) {
		ui.Eprintf("Playing...\n")
		err := audio.PlayAudioFile(path)
		if err == nil {
			ui.Eprintf("Done.\n")
			return nil
		}
		if errors.Is(err, audio.ErrPlaybackUnavailable) {
			ui.Eprintf("In-process playback unavailable, opening system player...\n")
			return openSystemDefault(path)
		}
		ui.Eprintf("Warning: audio playback failed: %v\n", err)
		return nil
	}

	return openSystemDefault(path)
}

// OpenWithSystemDefault opens path with the operating system's default
// application. Exported for callers that need the fallback used by PreviewFile.
func OpenWithSystemDefault(path string) error { return openSystemDefault(path) }

// openSystemDefault opens the file with the operating system's default handler.
func openSystemDefault(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(context.Background(), "open", path)
	case "linux":
		cmd = exec.CommandContext(context.Background(), "xdg-open", path)
	case "windows":
		cmd = exec.CommandContext(context.Background(), "cmd", "/c", "start", "", path)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open %s: %w", path, err)
	}
	return nil
}

// resizeImage scales an image to the given dimensions using Catmull-Rom
// interpolation for smooth results. Non-positive dimensions return src as-is.
func resizeImage(src image.Image, width, height int) image.Image {
	if width <= 0 || height <= 0 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

// detectImageExt guesses the file extension from image magic bytes.
func detectImageExt(data []byte) string {
	if len(data) < 4 {
		return ".bin"
	}
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return ".png"
	}
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return ".jpg"
	}
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return ".gif"
	}
	if data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 {
		return ".webp"
	}
	return ".bin"
}

// urlToFilename extracts a filename from a URL, falling back to a timestamp if
// the URL path doesn't have a usable name. Ensures the file extension matches.
func urlToFilename(rawURL, ext string) string {
	// Strip query params
	if idx := strings.Index(rawURL, "?"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	base := filepath.Base(rawURL)
	// Remove existing extension
	if e := filepath.Ext(base); e != "" {
		base = base[:len(base)-len(e)]
	}
	// If the base is empty or generic ("download", "image"), use a hash
	if base == "" || base == "." || base == "/" {
		base = fmt.Sprintf("image_%d", time.Now().Unix())
	}
	// Ensure unique name
	name := base + ext
	if _, err := os.Stat(name); err == nil {
		for i := 1; ; i++ {
			name = fmt.Sprintf("%s_%d%s", base, i, ext)
			if _, err := os.Stat(name); os.IsNotExist(err) {
				break
			}
		}
	}
	return name
}

// isAudioFile returns true if the file extension is a common audio format.
func isAudioFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".wav", ".mp3", ".flac", ".ogg", ".opus", ".aac", ".m4a", ".pcm":
		return true
	}
	return false
}

// isImageFile returns true if the file extension is a supported image type.
func isImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".jfif", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// mimeFromExt returns the MIME type for a given image extension.
func mimeFromExt(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg", ".jfif":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	}
	return ""
}
