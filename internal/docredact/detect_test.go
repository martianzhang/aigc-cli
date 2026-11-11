package docredact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectLegacyDOCAndDocumentPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.doc")
	if err := os.WriteFile(path, []byte{0xD0, 0xCF, 0x11, 0xE0}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Detect(path); err == nil || !strings.Contains(err.Error(), "legacy .doc") {
		t.Fatalf("Detect(.doc) error = %v, want legacy .doc message", err)
	}
	if !IsDocumentPath(path) {
		t.Fatal("IsDocumentPath(.doc) = false")
	}
	if IsDocumentPath("notes.txt") {
		t.Fatal("IsDocumentPath(.txt) = true")
	}
}

func TestDetectBMPAndTIFF(t *testing.T) {
	dir := t.TempDir()
	cases := map[string][]byte{
		"in.bmp":  {'B', 'M', 0, 0, 0, 0},
		"in.tif":  {'I', 'I', 0x2A, 0x00},
		"in.tiff": {'M', 'M', 0x00, 0x2A},
	}
	for name, head := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, head, 0o600); err != nil {
			t.Fatal(err)
		}
		format, err := Detect(path)
		if err != nil || format != FormatImage {
			t.Errorf("Detect(%s) = %v, %v; want image", name, format, err)
		}
	}
}
