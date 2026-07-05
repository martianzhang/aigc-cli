package service

import (
	"testing"
)

func TestExtractExt_mp4(t *testing.T) {
	if got := ExtractExt("https://example.com/video.mp4"); got != ".mp4" {
		t.Errorf("ExtractExt() = %q, want '.mp4'", got)
	}
}

func TestExtractExt_withQuery(t *testing.T) {
	if got := ExtractExt("https://example.com/video.mp4?token=abc"); got != ".mp4" {
		t.Errorf("ExtractExt() = %q, want '.mp4'", got)
	}
}

func TestExtractExt_noExt(t *testing.T) {
	if got := ExtractExt("https://example.com/video"); got != ".mp4" {
		t.Errorf("ExtractExt() = %q, want '.mp4'", got)
	}
}

func TestExtractExt_jpg(t *testing.T) {
	if got := ExtractExt("https://example.com/photo.jpg"); got != ".jpg" {
		t.Errorf("ExtractExt() = %q, want '.jpg'", got)
	}
}

func TestExtractExt_png(t *testing.T) {
	if got := ExtractExt("https://example.com/photo.PNG"); got != ".PNG" {
		t.Errorf("ExtractExt() = %q, want '.PNG'", got)
	}
}

func TestExtractExt_weirdPath(t *testing.T) {
	if got := ExtractExt("https://example.com/path/to/file.png?w=800&q=75"); got != ".png" {
		t.Errorf("ExtractExt() = %q, want '.png'", got)
	}
}

func TestDecodeBase64Image_PNG(t *testing.T) {
	// Minimal valid PNG (1x1 red pixel)
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xD7, 0x63, 0x60, 0x60, 0x60, 0x00,
		0x00, 0x00, 0x04, 0x00, 0x01, 0x27, 0x34, 0x27,
		0xAA, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
		0x44, 0xAE, 0x42, 0x60, 0x82,
	}

	b64 := toBase64(pngData)
	data, err := decodeBase64Image(b64)
	if err != nil {
		t.Fatalf("decodeBase64Image valid PNG: %v", err)
	}
	if len(data) == 0 {
		t.Error("decodeBase64Image returned empty data")
	}
}

func TestDecodeBase64Image_JPEG(t *testing.T) {
	// Minimal valid JPEG (SOI + some data)
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
		0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
		0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08,
		0x07, 0x07, 0x07, 0x09, 0x09, 0x08, 0x0A, 0x0C,
		0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D,
		0x1A, 0x1C, 0x1C, 0x20, 0x24, 0x2E, 0x27, 0x20,
		0x1C, 0x1C, 0x28, 0x37, 0x29, 0x2C, 0x30, 0x31,
		0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38,
		0x32, 0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0,
		0x00, 0x0B, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01,
		0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00,
		0x00, 0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B, 0xFF, 0xDA, 0x00, 0x08,
		0x01, 0x01, 0x00, 0x00, 0x3F, 0x00, 0xB2, 0x4B,
		0xE0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0xFF, 0xD9} // EOI

	b64 := toBase64(jpegData)
	data, err := decodeBase64Image(b64)
	if err != nil {
		t.Fatalf("decodeBase64Image valid JPEG: %v", err)
	}
	if len(data) == 0 {
		t.Error("decodeBase64Image returned empty data")
	}
}

func TestDecodeBase64Image_Invalid(t *testing.T) {
	b64 := toBase64([]byte{0x00, 0x00, 0x00, 0x00, 0x00})
	_, err := decodeBase64Image(b64)
	if err == nil {
		t.Error("decodeBase64Image invalid data: expected error")
	}
}

func TestIsLikelyImage_PNG(t *testing.T) {
	if !isLikelyImage([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}) {
		t.Error("isLikelyImage should detect PNG")
	}
}

func TestIsLikelyImage_JPEG(t *testing.T) {
	if !isLikelyImage([]byte{0xFF, 0xD8, 0xFF, 0xE0}) {
		t.Error("isLikelyImage should detect JPEG")
	}
}

func TestIsLikelyImage_GIF(t *testing.T) {
	if !isLikelyImage([]byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61}) {
		t.Error("isLikelyImage should detect GIF")
	}
}

func TestIsLikelyImage_WebP(t *testing.T) {
	// RIFF header + WEBP magic
	data := []byte{
		0x52, 0x49, 0x46, 0x46, // RIFF
		0x00, 0x00, 0x00, 0x00, // size
		0x57, 0x45, 0x42, 0x50, // WEBP
		0x00, // extra byte so len > 12
	}
	if !isLikelyImage(data) {
		t.Error("isLikelyImage should detect WebP")
	}
}

func TestIsLikelyImage_TooShort(t *testing.T) {
	if isLikelyImage([]byte{0x89, 0x50}) {
		t.Error("isLikelyImage should reject too-short data")
	}
}

func TestIsLikelyImage_NotImage(t *testing.T) {
	if isLikelyImage([]byte{0x00, 0x01, 0x02, 0x03}) {
		t.Error("isLikelyImage should reject unknown data")
	}
}

func TestIsLikelyImage_WebPTooShort(t *testing.T) {
	// RIFF header but too short for WEBP check
	data := []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00}
	if isLikelyImage(data) {
		t.Error("isLikelyImage should reject truncated WebP")
	}
}

func TestStripWhitespace_spaces(t *testing.T) {
	got := stripWhitespace("a b c")
	if got != "abc" {
		t.Errorf("stripWhitespace = %q, want 'abc'", got)
	}
}

func TestStripWhitespace_tabs(t *testing.T) {
	got := stripWhitespace("a\tb\tc")
	if got != "abc" {
		t.Errorf("stripWhitespace = %q, want 'abc'", got)
	}
}

func TestStripWhitespace_newlines(t *testing.T) {
	got := stripWhitespace("a\nb\nc")
	if got != "abc" {
		t.Errorf("stripWhitespace = %q, want 'abc'", got)
	}
}

func TestStripWhitespace_mixed(t *testing.T) {
	got := stripWhitespace(" a\tb\nc\r\n")
	if got != "abc" {
		t.Errorf("stripWhitespace = %q, want 'abc'", got)
	}
}

func TestStripWhitespace_empty(t *testing.T) {
	got := stripWhitespace("")
	if got != "" {
		t.Errorf("stripWhitespace = %q, want ''", got)
	}
}

func TestDecodeBase64Any_standard(t *testing.T) {
	data, err := decodeBase64Any("SGVsbG8=") // "Hello" in standard base64
	if err != nil {
		t.Fatalf("decodeBase64Any standard: %v", err)
	}
	if string(data) != "Hello" {
		t.Errorf("decodeBase64Any = %q, want 'Hello'", string(data))
	}
}

func TestDecodeBase64Any_raw(t *testing.T) {
	data, err := decodeBase64Any("SGVsbG8") // "Hello" without padding
	if err != nil {
		t.Fatalf("decodeBase64Any raw: %v", err)
	}
	if string(data) != "Hello" {
		t.Errorf("decodeBase64Any = %q, want 'Hello'", string(data))
	}
}

func TestDecodeBase64Any_autoPad(t *testing.T) {
	data, err := decodeBase64Any("aGVsbG8") // "hello" without padding
	if err != nil {
		t.Fatalf("decodeBase64Any auto-pad: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("decodeBase64Any = %q, want 'hello'", string(data))
	}
}

func TestDecodeBase64Any_whitespace(t *testing.T) {
	data, err := decodeBase64Any("SGVs\nbG 8=") // with whitespace
	if err != nil {
		t.Fatalf("decodeBase64Any whitespace: %v", err)
	}
	if string(data) != "Hello" {
		t.Errorf("decodeBase64Any = %q, want 'Hello'", string(data))
	}
}

// toBase64 encodes bytes to base64 string.
// Required because package-level base64 import is used by source but test can use it.
func toBase64(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result []byte
	i := 0
	for i < len(data) {
		var b [3]byte
		var n int
		for n = 0; n < 3 && i < len(data); n++ {
			b[n] = data[i]
			i++
		}
		if n == 1 {
			result = append(result, alphabet[b[0]>>2])
			result = append(result, alphabet[(b[0]&0x03)<<4])
			result = append(result, '=', '=')
		} else if n == 2 {
			result = append(result, alphabet[b[0]>>2])
			result = append(result, alphabet[(b[0]&0x03)<<4|b[1]>>4])
			result = append(result, alphabet[(b[1]&0x0F)<<2])
			result = append(result, '=')
		} else {
			result = append(result, alphabet[b[0]>>2])
			result = append(result, alphabet[(b[0]&0x03)<<4|b[1]>>4])
			result = append(result, alphabet[(b[1]&0x0F)<<2|b[2]>>6])
			result = append(result, alphabet[b[2]&0x3F])
		}
	}
	return string(result)
}
