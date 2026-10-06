package service

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func decodeDims(t *testing.T, data []byte) (int, int) {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestResizeImageBytes(t *testing.T) {
	large := encodePNG(t, 200, 100)

	t.Run("disabled returns input unchanged", func(t *testing.T) {
		got, resized, err := ResizeImageBytes(large, 0)
		if err != nil || resized {
			t.Fatalf("ResizeImageBytes(0) = (%v, %v), want (unchanged, false)", err, resized)
		}
		if !bytes.Equal(got, large) {
			t.Error("ResizeImageBytes(0) altered the bytes")
		}
	})

	t.Run("within max edge is untouched", func(t *testing.T) {
		got, resized, err := ResizeImageBytes(large, 400)
		if err != nil || resized {
			t.Fatalf("ResizeImageBytes(400) = (%v, %v), want (unchanged, false)", err, resized)
		}
		if !bytes.Equal(got, large) {
			t.Error("ResizeImageBytes(400) altered an image already within the max edge")
		}
	})

	t.Run("downscales longest edge and keeps aspect ratio", func(t *testing.T) {
		got, resized, err := ResizeImageBytes(large, 50)
		if err != nil || !resized {
			t.Fatalf("ResizeImageBytes(50) = (%v, %v), want (resized, true)", err, resized)
		}
		w, h := decodeDims(t, got)
		if w != 50 || h != 25 {
			t.Errorf("resized dims = %dx%d, want 50x25", w, h)
		}
	})

	t.Run("preserves png format", func(t *testing.T) {
		got, _, err := ResizeImageBytes(large, 50)
		if err != nil {
			t.Fatalf("ResizeImageBytes(50) error = %v", err)
		}
		if !bytes.HasPrefix(got, []byte("\x89PNG\r\n\x1a\n")) {
			t.Error("resized PNG lost its PNG signature")
		}
	})

	t.Run("never upscales", func(t *testing.T) {
		_, resized, err := ResizeImageBytes(encodePNG(t, 20, 20), 100)
		if err != nil || resized {
			t.Errorf("ResizeImageBytes(100) on 20x20 = (%v, %v), want (unchanged, false)", err, resized)
		}
	})

	t.Run("non-image rejected", func(t *testing.T) {
		if _, _, err := ResizeImageBytes([]byte("not an image"), 50); err == nil {
			t.Error("ResizeImageBytes(non-image) = nil error, want error")
		}
	})
}

func TestReadImageFile(t *testing.T) {
	dir := t.TempDir()
	path := writeImageFixture(t, dir, "ok.png", testPNG(t))

	data, err := ReadImageFile(path)
	if err != nil {
		t.Fatalf("ReadImageFile(%q) error = %v", path, err)
	}
	if len(data) == 0 {
		t.Error("ReadImageFile returned no bytes")
	}

	badPath := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(badPath, []byte("plain text"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := ReadImageFile(badPath); err == nil {
		t.Error("ReadImageFile(non-image) = nil error, want error")
	}
}
