package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageToDataURI(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "test.png")
	if err := os.WriteFile(png, []byte{0x89, 0x50, 0x4E, 0x47}, 0644); err != nil {
		t.Fatal(err)
	}
	uri, err := imageToDataURI(png)
	if err != nil {
		t.Fatalf("imageToDataURI failed: %v", err)
	}
	if !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Errorf("expected png data URI prefix, got: %s", uri[:30])
	}

	jpeg := filepath.Join(dir, "test.jpeg")
	if err := os.WriteFile(jpeg, []byte{0xFF, 0xD8}, 0644); err != nil {
		t.Fatal(err)
	}
	uri, err = imageToDataURI(jpeg)
	if err != nil {
		t.Fatalf("imageToDataURI failed: %v", err)
	}
	if !strings.HasPrefix(uri, "data:image/jpeg;base64,") {
		t.Errorf("expected jpeg data URI prefix, got: %s", uri[:30])
	}

	// 不存在的文件应报错
	if _, err := imageToDataURI(filepath.Join(dir, "none.png")); err == nil {
		t.Error("expected error for missing file")
	}
}
