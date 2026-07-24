package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEncodeImage_notFound(t *testing.T) {
	_, _, err := encodeImage("/nonexistent/path.png")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestEncodeImage_success(t *testing.T) {
	// Create a minimal valid PNG
	dir := t.TempDir()
	pngPath := filepath.Join(dir, "test.png")
	// Minimal 1x1 red PNG
	data := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG header
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, // IDAT chunk
		0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
		0x00, 0x00, 0x03, 0x00, 0x01, 0x26, 0xE0, 0xFE,
		0x69, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, // IEND chunk
		0x44, 0xAE, 0x42, 0x60, 0x82,
	}
	if err := os.WriteFile(pngPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	enc, mime, err := encodeImage(pngPath)
	if err != nil {
		t.Fatalf("encodeImage: %v", err)
	}
	if enc == "" {
		t.Error("expected non-empty base64 data")
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want %q", mime, "image/png")
	}
}

func TestOCRImage_nilProvider(t *testing.T) {
	_, err := OCRImage(nil, "test.png", "")
	if err == nil {
		t.Error("expected error for nil provider")
	}
}

func TestDescribeImage_nilProvider(t *testing.T) {
	_, err := DescribeImage(nil, "test.png", "desc")
	if err == nil {
		t.Error("expected error for nil provider")
	}
}

func TestMimeFromExt(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".png", "image/png"},
		{".webp", "image/webp"},
		{".gif", "image/gif"},
		{".bmp", "image/bmp"},
		{".unknown", "image/png"},
		{"", "image/png"},
	}
	for _, tt := range tests {
		got := mimeFromExt(tt.ext)
		if got != tt.want {
			t.Errorf("mimeFromExt(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestHasVersionSuffix(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://example.com/v1", true},
		{"http://example.com/v2", true},
		{"http://example.com/v1/", true},
		{"http://example.com/v10", true},
		{"http://example.com/", false},
		{"http://example.com", false},
		{"http://example.com/notaversion", false},
		{"http://example.com/v", false},
		{"", false},
	}
	for _, tt := range tests {
		got := hasVersionSuffix(tt.url)
		if got != tt.want {
			t.Errorf("hasVersionSuffix(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestOllamaModelKey(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"glm-ocr", "glm-ocr"},
		{"deepseek-ocr:latest", "deepseek-ocr"},
		{"x/flux2-klein:9b", "x"},
		{"gemma4", "gemma4"},
		{"", ""},
	}
	for _, tt := range tests {
		got := ollamaModelKey(tt.model)
		if got != tt.want {
			t.Errorf("ollamaModelKey(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}
