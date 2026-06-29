package service

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- isImageFile ---

func TestIsImageFile_png(t *testing.T) {
	if !isImageFile("photo.png") {
		t.Error("isImageFile('photo.png') should be true")
	}
}

func TestIsImageFile_jpg(t *testing.T) {
	if !isImageFile("photo.jpg") {
		t.Error("isImageFile('photo.jpg') should be true")
	}
	if !isImageFile("photo.jpeg") {
		t.Error("isImageFile('photo.jpeg') should be true")
	}
}

func TestIsImageFile_webp(t *testing.T) {
	if !isImageFile("photo.webp") {
		t.Error("isImageFile('photo.webp') should be true")
	}
}

func TestIsImageFile_bmp(t *testing.T) {
	if !isImageFile("photo.bmp") {
		t.Error("isImageFile('photo.bmp') should be true")
	}
}

func TestIsImageFile_gif(t *testing.T) {
	if !isImageFile("animation.gif") {
		t.Error("isImageFile('animation.gif') should be true")
	}
}

func TestIsImageFile_mp4(t *testing.T) {
	if isImageFile("video.mp4") {
		t.Error("isImageFile('video.mp4') should be false")
	}
}

func TestIsImageFile_noExt(t *testing.T) {
	if isImageFile("README") {
		t.Error("isImageFile('README') should be false")
	}
}

func TestIsImageFile_empty(t *testing.T) {
	if isImageFile("") {
		t.Error("isImageFile('') should be false")
	}
}

// --- mimeFromExt ---

func TestMimeFromExt_png(t *testing.T) {
	if got := mimeFromExt(".png"); got != "image/png" {
		t.Errorf("mimeFromExt('.png') = %q, want 'image/png'", got)
	}
}

func TestMimeFromExt_jpg(t *testing.T) {
	if got := mimeFromExt(".jpg"); got != "image/jpeg" {
		t.Errorf("mimeFromExt('.jpg') = %q, want 'image/jpeg'", got)
	}
	if got := mimeFromExt(".jpeg"); got != "image/jpeg" {
		t.Errorf("mimeFromExt('.jpeg') = %q, want 'image/jpeg'", got)
	}
}

func TestMimeFromExt_unknown(t *testing.T) {
	if got := mimeFromExt(".mp4"); got != "" {
		t.Errorf("mimeFromExt('.mp4') = %q, want ''", got)
	}
}

// --- resizeImage ---

func TestResizeImage_sameSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	dst := resizeImage(src, 10, 10)
	if dst.Bounds().Dx() != 10 || dst.Bounds().Dy() != 10 {
		t.Errorf("resizeImage same size = %dx%d, want 10x10", dst.Bounds().Dx(), dst.Bounds().Dy())
	}
}

func TestResizeImage_halfSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 50))
	dst := resizeImage(src, 50, 25)
	if dst.Bounds().Dx() != 50 || dst.Bounds().Dy() != 25 {
		t.Errorf("resizeImage half = %dx%d, want 50x25", dst.Bounds().Dx(), dst.Bounds().Dy())
	}
}

func TestResizeImage_zeroDimensions(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	dst := resizeImage(src, 0, 0)
	// Should not panic, should return some valid image
	if dst == nil {
		t.Error("resizeImage 0x0 returned nil")
	}
}

// --- PreviewFile ---

func TestPreviewFile_nonexistent(t *testing.T) {
	err := PreviewFile("/tmp/nonexistent_file_xyz_123")
	if err == nil {
		t.Error("PreviewFile on nonexistent file should return error")
	}
}

func TestPreviewFile_directory(t *testing.T) {
	dir := t.TempDir()
	err := PreviewFile(dir)
	if err == nil {
		t.Error("PreviewFile on directory should return error")
	}
}

func TestPreviewFile_inlineFallback(t *testing.T) {
	// Create a small PNG and verify PreviewFile doesn't crash
	// (inline display won't trigger since TERM_PROGRAM is not set)
	tmpFile := filepath.Join(t.TempDir(), "test.png")
	createTestPNG(t, tmpFile, 10, 10)

	// Should fall through to openSystemDefault
	// openSystemDefault will fail differently on different platforms,
	// but should return an error (no default handler in test env)
	err := PreviewFile(tmpFile)
	if err != nil {
		// Error from openSystemDefault is expected in CI/test env
		// Just verify it's not a "cannot access" or "is a directory" error
		if strings.Contains(err.Error(), "cannot access") || strings.Contains(err.Error(), "is a directory") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// --- isSixelCapableTerminal ---

func TestIsSixelCapableTerminal_default(t *testing.T) {
	// With no env vars set, should be false
	// Save and restore env
	oldTermProg := os.Getenv("TERM_PROGRAM")
	oldTerm := os.Getenv("TERM")
	oldWtSession := os.Getenv("WT_SESSION")
	defer func() {
		os.Setenv("TERM_PROGRAM", oldTermProg)
		os.Setenv("TERM", oldTerm)
		os.Setenv("WT_SESSION", oldWtSession)
	}()

	os.Unsetenv("TERM_PROGRAM")
	os.Unsetenv("TERM")
	os.Unsetenv("WT_SESSION")

	if isSixelCapableTerminal() {
		t.Error("isSixelCapableTerminal() should be false with default env")
	}
}

func TestIsSixelCapableTerminal_wezterm(t *testing.T) {
	old := os.Getenv("TERM_PROGRAM")
	defer os.Setenv("TERM_PROGRAM", old)
	os.Setenv("TERM_PROGRAM", "WezTerm")

	if !isSixelCapableTerminal() {
		t.Error("isSixelCapableTerminal() should be true for WezTerm")
	}
}

func TestIsSixelCapableTerminal_mintty(t *testing.T) {
	old := os.Getenv("TERM_PROGRAM")
	defer os.Setenv("TERM_PROGRAM", old)
	os.Setenv("TERM_PROGRAM", "mintty")

	if !isSixelCapableTerminal() {
		t.Error("isSixelCapableTerminal() should be true for mintty")
	}
}

func TestIsSixelCapableTerminal_wtSession(t *testing.T) {
	old := os.Getenv("WT_SESSION")
	defer os.Setenv("WT_SESSION", old)
	os.Setenv("WT_SESSION", "ws-123")

	if !isSixelCapableTerminal() {
		t.Error("isSixelCapableTerminal() should be true when WT_SESSION is set")
	}
}

func TestIsSixelCapableTerminal_xtermSixel(t *testing.T) {
	old := os.Getenv("TERM")
	defer os.Setenv("TERM", old)
	os.Setenv("TERM", "xterm-sixel")

	if !isSixelCapableTerminal() {
		t.Error("isSixelCapableTerminal() should be true for xterm-sixel")
	}
}

func TestIsSixelCapableTerminal_foot(t *testing.T) {
	old := os.Getenv("TERM")
	defer os.Setenv("TERM", old)
	os.Setenv("TERM", "foot")

	if !isSixelCapableTerminal() {
		t.Error("isSixelCapableTerminal() should be true for foot")
	}
}

// --- isInlineCapableTerminal ---

func TestIsInlineCapableTerminal_default(t *testing.T) {
	oldProg := os.Getenv("TERM_PROGRAM")
	oldKitty := os.Getenv("KITTY_WINDOW_ID")
	defer func() {
		os.Setenv("TERM_PROGRAM", oldProg)
		os.Setenv("KITTY_WINDOW_ID", oldKitty)
	}()
	os.Unsetenv("TERM_PROGRAM")
	os.Unsetenv("KITTY_WINDOW_ID")

	if isInlineCapableTerminal() {
		t.Error("isInlineCapableTerminal() should be false with default env")
	}
}

func TestIsInlineCapableTerminal_iTerm2(t *testing.T) {
	old := os.Getenv("TERM_PROGRAM")
	defer os.Setenv("TERM_PROGRAM", old)
	os.Setenv("TERM_PROGRAM", "iTerm.app")

	if !isInlineCapableTerminal() {
		t.Error("isInlineCapableTerminal() should be true for iTerm.app")
	}
}

func TestIsInlineCapableTerminal_kitty(t *testing.T) {
	old := os.Getenv("KITTY_WINDOW_ID")
	defer os.Setenv("KITTY_WINDOW_ID", old)
	os.Setenv("KITTY_WINDOW_ID", "1")

	if !isInlineCapableTerminal() {
		t.Error("isInlineCapableTerminal() should be true when KITTY_WINDOW_ID is set")
	}
}

// --- helpers ---

// createTestPNG creates a small PNG file for testing.
func createTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create test PNG: %v", err)
	}
	defer f.Close()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("failed to encode test PNG: %v", err)
	}
}
