package service

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	pngBytes  = []byte("\x89PNG\r\n\x1a\nfake-png-body")
	jpegBytes = []byte("\xff\xd8\xff\xe0fake-jpeg-body")
)

func writeImageFixture(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestImageToDataURI(t *testing.T) {
	dir := t.TempDir()
	pngPath := writeImageFixture(t, dir, "photo.png", pngBytes)
	jpgPath := writeImageFixture(t, dir, "photo.jpg", jpegBytes)
	wrongExtPath := writeImageFixture(t, dir, "photo.txt", pngBytes)

	tests := []struct {
		name string
		path string
		want string
	}{
		{"png", pngPath, "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)},
		{"jpg named jpeg", jpgPath, "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpegBytes)},
		{"png with wrong extension sniffs png", wrongExtPath, "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ImageToDataURI(tc.path)
			if err != nil {
				t.Fatalf("ImageToDataURI(%q) error = %v", tc.path, err)
			}
			if got != tc.want {
				t.Errorf("ImageToDataURI(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		_, err := ImageToDataURI(filepath.Join(dir, "missing.png"))
		if err == nil {
			t.Fatal("expected error for missing file")
		}
		if !strings.Contains(err.Error(), "failed to read image") {
			t.Errorf("error = %v, want wrapped read error", err)
		}
	})
}

func TestLocalFilesToDataURI(t *testing.T) {
	dir := t.TempDir()
	pngPath := writeImageFixture(t, dir, "local.png", pngBytes)

	const (
		url     = "https://example.com/image.png"
		dataURI = "data:image/gif;base64,R0lGOD"
		plain   = "not-a-file"
	)
	inputs := []string{url, dataURI, plain, pngPath}
	original := append([]string(nil), inputs...)

	got, err := LocalFilesToDataURI(inputs)
	if err != nil {
		t.Fatalf("LocalFilesToDataURI() error = %v", err)
	}

	want := []string{url, dataURI, plain, "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	for i := range original {
		if inputs[i] != original[i] {
			t.Errorf("input slice mutated at %d: %q, want %q", i, inputs[i], original[i])
		}
	}
}
