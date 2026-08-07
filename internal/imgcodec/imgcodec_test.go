package imgcodec

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// testImage 生成一张 64x48 的渐变测试图（带 JPEG/PNG 真实感纹理）。
func testImage() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.NRGBA{
				R: uint8(x * 4),
				G: uint8(y * 5),
				B: uint8((x + y) * 2),
				A: 255,
			})
		}
	}
	return img
}

// encodePNG 生成标准 PNG 字节，用于解码路径测试。
func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestSniffImageExt(t *testing.T) {
	img := testImage()
	pngData := encodePNG(t, img)

	var jpgBuf bytes.Buffer
	if err := jpeg.Encode(&jpgBuf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	jpgData := jpgBuf.Bytes()

	webpData := mustEncode(t, img, "webp", 75)
	avifData := mustEncode(t, img, "avif", 60)
	jxlData := mustEncode(t, img, "jxl", 75)

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"png", pngData, ".png"},
		{"jpeg", jpgData, ".jpg"},
		{"webp", webpData, ".webp"},
		{"avif", avifData, ".avif"},
		{"jxl", jxlData, ".jxl"},
		{"empty", nil, ""},
		{"short", []byte{0x89, 0x50}, ""},
		{"random", []byte("hello world this is not an image at all"), ""},
		{"gif", append([]byte("GIF89a"), make([]byte, 32)...), ".gif"},
		{"bmp", append([]byte("BM"), make([]byte, 32)...), ".bmp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SniffImageExt(tt.data); got != tt.want {
				t.Errorf("SniffImageExt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func mustEncode(t *testing.T, img image.Image, format string, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := Encode(&buf, img, format, quality); err != nil {
		t.Fatalf("Encode(%s): %v", format, err)
	}
	if buf.Len() == 0 {
		t.Fatalf("Encode(%s): empty output", format)
	}
	return buf.Bytes()
}

func TestEncodeFormats(t *testing.T) {
	img := testImage()
	formats := []string{"jpg", "jpeg", "webp", "avif", "jxl", "png"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			data := mustEncode(t, img, format, 70)
			if len(data) < 10 {
				t.Fatalf("output too small: %d bytes", len(data))
			}
		})
	}
}

func TestEncodeInvalidFormat(t *testing.T) {
	var buf bytes.Buffer
	if _, err := Encode(&buf, testImage(), "gif", 70); err == nil {
		t.Fatal("expected error for unsupported format gif")
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	img := testImage()

	// 各 wasm 编码器输出的数据，必须能被 image.Decode 读回
	formats := []string{"png", "jpg", "webp", "avif", "jxl"}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			data := mustEncode(t, img, format, 70)
			decoded, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("image.Decode(%s): %v", format, err)
			}
			b := decoded.Bounds()
			if b.Dx() != 64 || b.Dy() != 48 {
				t.Errorf("decoded size = %dx%d, want 64x48", b.Dx(), b.Dy())
			}
		})
	}
}

func TestEncodeToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.avif")
	n, err := EncodeToFile(testImage(), path, "avif", 60)
	if err != nil {
		t.Fatalf("EncodeToFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if n != info.Size() {
		t.Errorf("returned size %d != file size %d", n, info.Size())
	}
	if n == 0 {
		t.Error("expected non-zero file size")
	}
}
