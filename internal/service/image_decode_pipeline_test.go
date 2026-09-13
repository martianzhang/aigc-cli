package service

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tinyPNG is a 1x1 transparent PNG used as a known-good image payload.
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func writeTempImage(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(tinyPNG)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, raw, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

func TestDecodeImageURLsInline_when_given_various_sources(t *testing.T) {
	dir := t.TempDir()
	pngPath := writeTempImage(t, dir, "real.png")
	b64Path := filepath.Join(dir, "b64.txt")
	if err := os.WriteFile(b64Path, []byte(tinyPNG), 0644); err != nil {
		t.Fatalf("write base64 fixture: %v", err)
	}

	tests := []struct {
		name       string
		input      string
		wantPrefix string
	}{
		{name: "remote URL", input: "https://example.com/a.png", wantPrefix: "https://example.com/a.png"},
		{name: "real image file", input: pngPath, wantPrefix: pngPath},
		{name: "base64 text file", input: b64Path, wantPrefix: "data:image/png;base64,"},
		{name: "unknown string", input: "/no/such/file.png", wantPrefix: "/no/such/file.png"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeImageURLsInline([]string{tc.input}, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("want 1 result, got %d", len(got))
			}
			if !strings.HasPrefix(got[0], tc.wantPrefix) {
				t.Errorf("want prefix %q, got %q", tc.wantPrefix, got[0])
			}
		})
	}
}

func TestResolveImageSource_when_given_various_sources(t *testing.T) {
	dir := t.TempDir()
	pngPath := writeTempImage(t, dir, "real.png")

	t.Run("remote URL passes through", func(t *testing.T) {
		got, created, err := ResolveImageSource("https://example.com/a.png", "", dir)
		if err != nil || created || got != "https://example.com/a.png" {
			t.Fatalf("got (%q, created=%v, err=%v)", got, created, err)
		}
	})

	t.Run("real image with no conversion passes through", func(t *testing.T) {
		got, created, err := ResolveImageSource(pngPath, "", dir)
		if err != nil || created || got != pngPath {
			t.Fatalf("got (%q, created=%v, err=%v)", got, created, err)
		}
	})

	t.Run("data URI is decoded to a new file", func(t *testing.T) {
		got, created, err := ResolveImageSource("data:image/png;base64,"+tinyPNG, "", dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !created {
			t.Fatal("want created=true")
		}
		if _, statErr := os.Stat(got); statErr != nil {
			t.Fatalf("output not written: %v", statErr)
		}
	})

	t.Run("unsupported target format errors", func(t *testing.T) {
		if _, _, err := ResolveImageSource("data:image/png;base64,"+tinyPNG, "tiff", dir); err == nil {
			t.Fatal("want error for unsupported target format")
		}
	})

	t.Run("converting a real image onto itself errors", func(t *testing.T) {
		if _, _, err := ResolveImageSource(pngPath, "png", dir); err == nil {
			t.Fatal("want overwrite-source error")
		}
	})
}
