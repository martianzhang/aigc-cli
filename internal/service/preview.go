package service

import (
	"encoding/base64"
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

	"github.com/mattn/go-sixel"
	_ "golang.org/x/image/webp"
)

// PreviewFile opens a file or URL with the system default application.
// For image files, it also attempts inline terminal display when the
// terminal supports it (Kitty, iTerm2, or Sixel).
// URLs are downloaded to a temporary file first.
func PreviewFile(path string) error {
	// Detect URL — download to temp file first
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		data, err := FetchImage(path)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", path, err)
		}
		// Detect image extension from magic bytes
		ext := detectImageExt(data)
		// Generate a filename from the URL
		saveName := urlToFilename(path, ext)
		if err := os.WriteFile(saveName, data, 0644); err != nil {
			return fmt.Errorf("failed to save %s: %w", saveName, err)
		}
		fmt.Printf("Saved: %s\n", saveName)
		path = saveName
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}

	// Try inline terminal image display for supported terminals
	if isImageFile(path) {
		if trySixelImage(path) {
			return nil
		}
		if tryInlineImage(path) {
			return nil
		}
	}

	return openSystemDefault(path)
}

// openSystemDefault opens the file with the operating system's default handler.
func openSystemDefault(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open %s: %w", path, err)
	}
	return nil
}

// trySixelImage attempts to display an image inline using the Sixel protocol.
// Returns true if successful (terminal supports sixel and encoding succeeded).
func trySixelImage(path string) bool {
	if !isSixelCapableTerminal() {
		return false
	}

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return false
	}

	// Get terminal width for sizing
	termWidth := 80
	imgBounds := img.Bounds()
	imgW := imgBounds.Dx()
	imgH := imgBounds.Dy()

	// Scale to fit terminal width while maintaining aspect ratio
	if imgW > termWidth*8 { // sixel uses 8 pixels per terminal column
		scale := float64(termWidth*8) / float64(imgW)
		newW := int(float64(imgW) * scale)
		newH := int(float64(imgH) * scale)
		img = resizeImage(img, newW, newH)
	}

	enc := sixel.NewEncoder(os.Stdout)
	enc.Width = img.Bounds().Dx()
	enc.Height = img.Bounds().Dy()

	if err := enc.Encode(img); err != nil {
		return false
	}
	return true
}

// resizeImage scales an image to the given dimensions using nearest-neighbor.
func resizeImage(src image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			sx := x * srcW / width
			sy := y * srcH / height
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// tryInlineImage attempts to display an image inline in the terminal
// using the iTerm2 inline image protocol. Returns true if successful.
func tryInlineImage(path string) bool {
	if !isInlineCapableTerminal() {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	ext := strings.ToLower(filepath.Ext(path))
	mime := mimeFromExt(ext)
	if mime == "" {
		return false
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	fmt.Printf("\033]1337;File=inline=1;preserveAspectRatio=1;mimeType=%s:%s\a\n", mime, encoded)
	return true
}

// isSixelCapableTerminal checks if the current terminal supports Sixel.
func isSixelCapableTerminal() bool {
	// WezTerm
	if os.Getenv("TERM_PROGRAM") == "WezTerm" {
		return true
	}
	// mintty (Git Bash on Windows)
	if os.Getenv("TERM_PROGRAM") == "mintty" {
		return true
	}
	// xterm with explicit sixel support
	if strings.Contains(os.Getenv("TERM"), "sixel") {
		return true
	}
	// Windows Terminal (has WT_SESSION, supports sixel since v1.22+)
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	// foot terminal
	if os.Getenv("TERM") == "foot" {
		return true
	}
	return false
}

// isInlineCapableTerminal checks if the current terminal supports iTerm2/Kitty inline images.
func isInlineCapableTerminal() bool {
	// iTerm2 sets TERM_PROGRAM=iTerm.app
	if os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		return true
	}
	// Kitty sets KITTY_WINDOW_ID
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	return false
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
