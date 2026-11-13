package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-sixel"
	"golang.org/x/term"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

const (
	// EnvNoInlinePreview turns inline terminal rendering off, so `preview` and
	// `--preview` always hand the file to the system viewer. Mirrors
	// terminal.EnvDisable (AIGC_CLI_NO_TITLE) for the title/progress escapes.
	EnvNoInlinePreview = "AIGC_CLI_NO_INLINE_PREVIEW"

	// maxInlinePayloadBytes caps the image payload embedded in an inline-image
	// escape. Terminals and multiplexers truncate very long escapes, and once
	// truncated the rest of the base64 is printed as literal text. Real terminal
	// parsers also stall on multi-megabyte OSC strings, so the preview is
	// re-encoded small (or skipped in favour of the system viewer) past this cap.
	maxInlinePayloadBytes = 512 * 1024

	// previewJPEGQuality compresses the re-encoded preview. It is a screen
	// thumbnail, so a moderate quality keeps the payload small.
	previewJPEGQuality = 85
)

// previewInlineWriter receives inline-image escapes. It is a variable so tests
// can capture the bytes instead of writing them to the test runner's stdout.
var previewInlineWriter io.Writer = os.Stdout

// previewStdoutIsTerminal reports whether stdout is a real terminal. Inline
// escapes are meaningless anywhere else, so the probe is a variable to keep the
// decision testable without allocating a pty (no platform-specific API).
var previewStdoutIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// inlinePreviewAllowed reports whether an inline-image escape may be written.
// stdout must be a terminal — a pipe, a redirected log, or output captured by
// an IDE/TUI would otherwise receive the escape as literal base64 text — and the
// user must not have opted out via AIGC_CLI_NO_INLINE_PREVIEW.
func inlinePreviewAllowed() bool {
	if os.Getenv(EnvNoInlinePreview) != "" {
		return false
	}
	return previewStdoutIsTerminal()
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

	img, _, err := imgcodec.Decode(f)
	if err != nil {
		return false
	}

	// Scale the image to the preview budget derived from the current terminal
	// (a fraction of the window, see previewFillRatio). When the terminal size
	// is unknown the image is left untouched.
	img = fitImageToTerminal(img)

	enc := sixel.NewEncoder(previewInlineWriter)
	enc.Width = img.Bounds().Dx()
	enc.Height = img.Bounds().Dy()

	if err := enc.Encode(img); err != nil {
		return false
	}
	return true
}

// tryInlineImage displays an image inline using the iTerm2/Kitty OSC 1337
// protocol. The payload is never the original file: buildInlinePayload hands
// back a preview-sized copy, so the escape stays small.
func tryInlineImage(path string) bool {
	if !isInlineCapableTerminal() {
		return false
	}

	payload, mime, ok := buildInlinePayload(path)
	if !ok {
		return false
	}

	encoded := base64.StdEncoding.EncodeToString(payload)
	// Pass the payload's own pixel dimensions. With a bare `inline=1` iTerm2
	// draws at native size and, on HiDPI, can fill the window; with cell units
	// the reserved area (cells x cell-height) rarely matches the image aspect
	// exactly, leaving a blank band under it. Pixel units reserve exactly the
	// image rectangle, so the preview hugs the content.
	header := fmt.Sprintf("\033]1337;File=inline=1;preserveAspectRatio=1;mimeType=%s:", mime)
	if w, h, ok := previewPayloadPixels(payload); ok {
		header = fmt.Sprintf("\033]1337;File=inline=1;preserveAspectRatio=1;width=%dpx;height=%dpx;mimeType=%s:", w, h, mime)
	}
	// Start on a fresh line before the protocol escape, so the image renders
	// at a predictable position regardless of prior text output on stdout.
	if _, err := fmt.Fprintf(previewInlineWriter, "\n%s%s\a\n", header, encoded); err != nil {
		return false
	}
	return true
}

// previewPayloadPixels returns the pixel dimensions of an already preview-sized
// payload. The inline escape requests exactly this rectangle, so the terminal
// reserves no extra rows (which show up as blank/black bands under the image).
func previewPayloadPixels(payload []byte) (width, height int, ok bool) {
	cfg, _, err := imgcodec.DecodeConfig(bytes.NewReader(payload))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// buildInlinePayload returns the image bytes to embed in an inline-image escape
// along with their MIME type. A file that is already small enough is sent
// verbatim; anything bigger is downscaled to the preview budget and re-encoded
// (JPEG unless the image has transparency, which PNG preserves). ok is false
// when the payload cannot be made small enough, so the caller falls back to the
// system viewer instead of dumping base64 onto the screen.
func buildInlinePayload(path string) (payload []byte, mime string, ok bool) {
	ext := strings.ToLower(filepath.Ext(path))
	extMime := mimeFromExt(ext)
	if extMime == "" {
		return nil, "", false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", false
	}

	// Animated GIFs must stay byte-identical: re-encoding keeps a single frame.
	if ext == ".gif" {
		if len(data) > maxInlinePayloadBytes {
			return nil, "", false
		}
		return data, extMime, true
	}

	cfg, _, err := imgcodec.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", false
	}
	_, _, needsResize := previewInlineSize(cfg.Width, cfg.Height)
	if !needsResize && len(data) <= maxInlinePayloadBytes {
		return data, extMime, true
	}

	img, err := decodeImageSafe(data)
	if err != nil {
		return nil, "", false
	}
	// Never enlarges: an image inside the budget keeps its own dimensions.
	img = fitImageToTerminal(img)

	format, formatMime := "jpg", "image/jpeg"
	if hasTransparency(img) {
		format, formatMime = "png", "image/png"
	}
	var buf bytes.Buffer
	if _, err := imgcodec.Encode(&buf, img, format, previewJPEGQuality); err != nil {
		return nil, "", false
	}
	if buf.Len() > maxInlinePayloadBytes {
		return nil, "", false
	}
	return buf.Bytes(), formatMime, true
}

// hasTransparency reports whether any pixel is not fully opaque. The scan stops
// at the first transparent pixel; it only runs on preview-sized images, since
// callers re-encode after downscaling.
func hasTransparency(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return true
			}
		}
	}
	return false
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
