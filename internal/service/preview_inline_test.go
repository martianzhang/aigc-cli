package service

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

// setPreviewStdoutTerminal injects the TTY probe so tests never need a pty.
func setPreviewStdoutTerminal(t *testing.T, isTerminal bool) {
	t.Helper()
	prev := previewStdoutIsTerminal
	t.Cleanup(func() { previewStdoutIsTerminal = prev })
	previewStdoutIsTerminal = func() bool { return isTerminal }
}

// captureInlineWriter redirects inline escapes into a buffer.
func captureInlineWriter(t *testing.T) *bytes.Buffer {
	t.Helper()
	prev := previewInlineWriter
	buf := &bytes.Buffer{}
	t.Cleanup(func() { previewInlineWriter = prev })
	previewInlineWriter = buf
	return buf
}

func mustWritePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("png encode %s: %v", path, err)
	}
}

// noiseImage returns a deterministic RGBA image whose pixels barely compress,
// so a PNG of it stays large even at preview dimensions.
func noiseImage(width, height int, alpha uint8) *image.RGBA {
	rng := rand.New(rand.NewSource(1))
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(rng.Intn(256))
		img.Pix[i+1] = uint8(rng.Intn(256))
		img.Pix[i+2] = uint8(rng.Intn(256))
		img.Pix[i+3] = alpha
	}
	return img
}

func TestInlinePreviewAllowed(t *testing.T) {
	cases := []struct {
		name string
		env  string
		tty  bool
		want bool
	}{
		{name: "terminal", env: "", tty: true, want: true},
		{name: "not a terminal", env: "", tty: false, want: false},
		{name: "opted out", env: "1", tty: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvNoInlinePreview, tc.env)
			setPreviewStdoutTerminal(t, tc.tty)
			if got := inlinePreviewAllowed(); got != tc.want {
				t.Errorf("inlinePreviewAllowed() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInlinePreviewDisabledByFlag(t *testing.T) {
	setPreviewStdoutTerminal(t, true)
	t.Cleanup(func() { SetInlinePreviewDisabled(false) })

	SetInlinePreviewDisabled(true)
	if inlinePreviewAllowed() {
		t.Error("inlinePreviewAllowed() = true after SetInlinePreviewDisabled(true)")
	}
	if InlinePreviewAvailable() {
		t.Error("InlinePreviewAvailable() = true after SetInlinePreviewDisabled(true)")
	}

	SetInlinePreviewDisabled(false)
	if !inlinePreviewAllowed() {
		t.Error("inlinePreviewAllowed() = false after re-enabling")
	}
}

func TestBuildInlinePayloadSmallPNGPassthrough(t *testing.T) {
	path := filepath.Join(t.TempDir(), "small.png")
	createTestPNG(t, path)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read png: %v", err)
	}

	payload, mime, ok := buildInlinePayload(path)
	if !ok {
		t.Fatal("buildInlinePayload(small png) not ok")
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png", mime)
	}
	if !bytes.Equal(payload, want) {
		t.Error("small png should be sent verbatim")
	}
}

func TestBuildInlinePayloadDownscalesOpaqueToJPEG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.png")
	img := image.NewRGBA(image.Rect(0, 0, 1600, 1200))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 0x80, 0x80, 0x80, 0xff
	}
	mustWritePNG(t, path, img)

	payload, mime, ok := buildInlinePayload(path)
	if !ok {
		t.Fatal("buildInlinePayload(opaque png) not ok")
	}
	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg (opaque preview re-encodes as JPEG)", mime)
	}
	cfg, _, err := imgcodec.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode preview payload: %v", err)
	}
	if cfg.Width > maxPreviewWidthPx || cfg.Height > maxPreviewHeightPx {
		t.Errorf("preview payload = %dx%d, want <= %dx%d", cfg.Width, cfg.Height, maxPreviewWidthPx, maxPreviewHeightPx)
	}
	if len(payload) > maxInlinePayloadBytes {
		t.Errorf("payload = %d bytes, want <= %d", len(payload), maxInlinePayloadBytes)
	}
}

func TestBuildInlinePayloadKeepsTransparency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alpha.png")
	img := noiseImage(1600, 1200, 0x80)
	mustWritePNG(t, path, img)

	payload, mime, ok := buildInlinePayload(path)
	if !ok {
		t.Fatal("buildInlinePayload(transparent png) not ok")
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png (transparency must survive)", mime)
	}
	cfg, _, err := imgcodec.DecodeConfig(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode preview payload: %v", err)
	}
	if cfg.Width > maxPreviewWidthPx || cfg.Height > maxPreviewHeightPx {
		t.Errorf("preview payload = %dx%d, want <= %dx%d", cfg.Width, cfg.Height, maxPreviewWidthPx, maxPreviewHeightPx)
	}
}

func TestBuildInlinePayloadReencodesOversizedWithinBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "noise.png")
	// 600x460 fits the preview budget, so only the payload size forces a
	// re-encode: a screen-sized PNG of a photo can be far bigger than the cap.
	img := noiseImage(600, 460, 0xff)
	mustWritePNG(t, path, img)
	original, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat png: %v", err)
	}
	if original.Size() <= maxInlinePayloadBytes {
		t.Fatalf("fixture PNG = %d bytes, want > %d", original.Size(), maxInlinePayloadBytes)
	}

	payload, mime, ok := buildInlinePayload(path)
	if !ok {
		t.Fatal("oversized preview payload should be shrunk, not rejected")
	}
	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg", mime)
	}
	if len(payload) > maxInlinePayloadBytes {
		t.Errorf("payload = %d bytes, want <= %d", len(payload), maxInlinePayloadBytes)
	}
}

func TestBuildInlinePayloadGIFPassthrough(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anim.gif")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create gif: %v", err)
	}
	palette := color.Palette{color.Black, color.White}
	if err := gif.Encode(f, image.NewPaletted(image.Rect(0, 0, 10, 10), palette), nil); err != nil {
		f.Close()
		t.Fatalf("gif encode: %v", err)
	}
	f.Close()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read gif: %v", err)
	}

	payload, mime, ok := buildInlinePayload(path)
	if !ok {
		t.Fatal("buildInlinePayload(gif) not ok")
	}
	if mime != "image/gif" {
		t.Errorf("mime = %q, want image/gif", mime)
	}
	if !bytes.Equal(payload, want) {
		t.Error("gif must be sent verbatim so animation survives")
	}
}

func TestBuildInlinePayloadRejectsOversizedGIF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.gif")
	data := append([]byte("GIF89a"), bytes.Repeat([]byte{0}, maxInlinePayloadBytes)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write gif: %v", err)
	}

	if _, _, ok := buildInlinePayload(path); ok {
		t.Error("a payload above the cap must fall back to the system viewer")
	}
}

func TestBuildInlinePayloadUnsupportedExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, _, ok := buildInlinePayload(path); ok {
		t.Error("non-image extensions must not be embedded inline")
	}
}

func TestTryInlineImageWritesOSC1337(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("KITTY_WINDOW_ID", "")
	buf := captureInlineWriter(t)

	path := filepath.Join(t.TempDir(), "small.png")
	createTestPNG(t, path)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read png: %v", err)
	}

	if !tryInlineImage(path) {
		t.Fatal("tryInlineImage with a capable terminal should succeed")
	}
	out := buf.String()
	w, h, ok := previewPayloadPixels(want)
	if !ok {
		t.Fatal("previewPayloadPixels(payload) = !ok")
	}
	prefix := fmt.Sprintf("\n\033]1337;File=inline=1;preserveAspectRatio=1;width=%dpx;height=%dpx;mimeType=image/png:", w, h)
	if !strings.HasPrefix(out, prefix) {
		t.Fatalf("output does not start with the inline-image header: %q", out[:min(len(out), 80)])
	}
	if !strings.HasSuffix(out, "\a\n") {
		t.Fatal("output must end with the BEL terminator")
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(out, prefix), "\a\n")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("payload is not base64: %v", err)
	}
	if !bytes.Equal(decoded, want) {
		t.Error("decoded payload does not match the source image")
	}
}

func TestTryInlineImageSkipsIncapableTerminal(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "xterm")
	t.Setenv("KITTY_WINDOW_ID", "")
	buf := captureInlineWriter(t)

	path := filepath.Join(t.TempDir(), "small.png")
	createTestPNG(t, path)

	if tryInlineImage(path) {
		t.Error("tryInlineImage must not claim success on an incapable terminal")
	}
	if buf.Len() != 0 {
		t.Errorf("nothing should be written, got %d bytes", buf.Len())
	}
}

func TestHasTransparency(t *testing.T) {
	opaque := noiseImage(4, 4, 0xff)
	if hasTransparency(opaque) {
		t.Error("fully opaque image reported as transparent")
	}
	translucent := noiseImage(4, 4, 0xff)
	translucent.Pix[7] = 0x00 // alpha of pixel 1
	if !hasTransparency(translucent) {
		t.Error("translucent pixel was not detected")
	}
}

func TestPreviewPayloadPixelsMatchResizedImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.png")
	img := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, _, ok := buildInlinePayload(path)
	if !ok {
		t.Fatal("buildInlinePayload() = !ok")
	}
	w, h, ok := previewPayloadPixels(payload)
	if !ok || w <= 0 || h <= 0 {
		t.Fatalf("previewPayloadPixels() = (%d,%d,%v)", w, h, ok)
	}
	if w > maxPreviewWidthPx || h > maxPreviewHeightPx {
		t.Fatalf("payload %dx%d exceeds preview caps", w, h)
	}
}
