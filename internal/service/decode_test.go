package service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"
)

// validPNG is a real 1x1 image encoded by the standard library,
// guaranteed to be decodable by image.Decode for format conversion tests.
var validPNG = func() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

func TestDecodeImageInput_dataURIPNG(t *testing.T) {
	content := []byte("data:image/png;base64," + toBase64(validPNG))
	data, ext, err := DecodeImageInput(content, "")
	if err != nil {
		t.Fatalf("DecodeImageInput data URI PNG: %v", err)
	}
	if ext != ".png" {
		t.Errorf("ext = %q, want .png", ext)
	}
	if !bytes.Equal(data, validPNG) {
		t.Error("decoded bytes differ from source PNG")
	}
}

func TestDecodeImageInput_dataURIJPEG(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00}
	content := []byte("data:image/jpeg;base64," + toBase64(jpeg))
	data, ext, err := DecodeImageInput(content, "")
	if err != nil {
		t.Fatalf("DecodeImageInput data URI JPEG: %v", err)
	}
	if ext != ".jpg" {
		t.Errorf("ext = %q, want .jpg", ext)
	}
	if !bytes.HasPrefix(data, []byte{0xFF, 0xD8}) {
		t.Error("decoded data does not start with JPEG magic")
	}
}

func TestDecodeImageInput_rawBase64(t *testing.T) {
	content := []byte(toBase64(validPNG))
	data, ext, err := DecodeImageInput(content, "")
	if err != nil {
		t.Fatalf("DecodeImageInput raw base64: %v", err)
	}
	if ext != ".png" {
		t.Errorf("ext = %q, want .png", ext)
	}
	if !bytes.Equal(data, validPNG) {
		t.Error("decoded bytes differ from source PNG")
	}
}

func TestDecodeImageInput_alreadyImage(t *testing.T) {
	data, ext, err := DecodeImageInput(validPNG, "")
	if err != nil {
		t.Fatalf("DecodeImageInput already image: %v", err)
	}
	if ext != ".png" {
		t.Errorf("ext = %q, want .png", ext)
	}
	if !bytes.Equal(data, validPNG) {
		t.Error("image bytes should pass through unchanged")
	}
}

func TestDecodeImageInput_convertToJPG(t *testing.T) {
	data, ext, err := DecodeImageInput(validPNG, "jpg")
	if err != nil {
		t.Fatalf("DecodeImageInput convert to jpg: %v", err)
	}
	if ext != ".jpg" {
		t.Errorf("ext = %q, want .jpg", ext)
	}
	if !bytes.HasPrefix(data, []byte{0xFF, 0xD8}) {
		t.Error("converted data does not start with JPEG magic")
	}
}

func TestDecodeImageInput_convertJpegAlias(t *testing.T) {
	data, ext, err := DecodeImageInput(validPNG, "jpeg")
	if err != nil {
		t.Fatalf("DecodeImageInput convert with jpeg alias: %v", err)
	}
	if ext != ".jpg" {
		t.Errorf("ext = %q, want .jpg (jpeg normalized to jpg)", ext)
	}
	if !bytes.HasPrefix(data, []byte{0xFF, 0xD8}) {
		t.Error("converted data does not start with JPEG magic")
	}
}

func TestDecodeImageInput_convertToWebP(t *testing.T) {
	data, ext, err := DecodeImageInput(validPNG, "webp")
	if err != nil {
		t.Fatalf("DecodeImageInput convert to webp: %v", err)
	}
	if ext != ".webp" {
		t.Errorf("ext = %q, want .webp", ext)
	}
	if !strings.HasPrefix(string(data), "RIFF") {
		t.Error("converted data does not start with RIFF (webp magic)")
	}
}

func TestDecodeImageInput_sameFormatNoOp(t *testing.T) {
	data, ext, err := DecodeImageInput(validPNG, "png")
	if err != nil {
		t.Fatalf("DecodeImageInput same format: %v", err)
	}
	if ext != ".png" {
		t.Errorf("ext = %q, want .png", ext)
	}
	if !bytes.Equal(data, validPNG) {
		t.Error("same-format conversion should return original bytes")
	}
}

func TestDecodeImageInput_invalidText(t *testing.T) {
	_, _, err := DecodeImageInput([]byte("this is not an image"), "")
	if err == nil {
		t.Error("DecodeImageInput invalid text: expected error")
	}
}

func TestDecodeImageInput_empty(t *testing.T) {
	_, _, err := DecodeImageInput([]byte(""), "")
	if err == nil {
		t.Error("DecodeImageInput empty input: expected error")
	}
}

func TestDecodeImageInput_malformedDataURI(t *testing.T) {
	_, _, err := DecodeImageInput([]byte("data:image/png;base64"), "")
	if err == nil {
		t.Error("DecodeImageInput malformed data URI (no comma): expected error")
	}
}

func TestDecodeImageInput_unsupportedTarget(t *testing.T) {
	_, _, err := DecodeImageInput(validPNG, "bmp")
	if err == nil {
		t.Error("DecodeImageInput unsupported target format: expected error")
	}
}

func TestDecodeImageInput_base64Output(t *testing.T) {
	data, ext, err := DecodeImageInput(validPNG, "base64")
	if err != nil {
		t.Fatalf("DecodeImageInput base64 output: %v", err)
	}
	if ext != ".txt" {
		t.Errorf("ext = %q, want .txt", ext)
	}
	// Base64 text must decode back to the original image bytes.
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		t.Fatalf("output is not valid base64: %v", err)
	}
	if !bytes.Equal(decoded, validPNG) {
		t.Error("base64 output does not round-trip to original image")
	}
}

func TestDecodeImageInput_base64OutputFromDataURI(t *testing.T) {
	content := []byte("data:image/png;base64," + toBase64(validPNG))
	data, _, err := DecodeImageInput(content, "base64")
	if err != nil {
		t.Fatalf("DecodeImageInput base64 from data URI: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		t.Fatalf("output is not valid base64: %v", err)
	}
	if !bytes.Equal(decoded, validPNG) {
		t.Error("base64 output does not round-trip to original image")
	}
}

func TestDecodeImageInput_dataURTOutput(t *testing.T) {
	data, ext, err := DecodeImageInput(validPNG, "datauri")
	if err != nil {
		t.Fatalf("DecodeImageInput datauri output: %v", err)
	}
	if ext != ".txt" {
		t.Errorf("ext = %q, want .txt", ext)
	}
	prefix := "data:image/png;base64,"
	if !strings.HasPrefix(string(data), prefix) {
		t.Errorf("output does not start with %q, got %.40s", prefix, data)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(data)[len(prefix):])
	if err != nil {
		t.Fatalf("payload is not valid base64: %v", err)
	}
	if !bytes.Equal(decoded, validPNG) {
		t.Error("data URI payload does not round-trip to original image")
	}
}

func TestDecodeImageInput_dataURTAlias(t *testing.T) {
	data, _, err := DecodeImageInput(validPNG, "data-uri")
	if err != nil {
		t.Fatalf("DecodeImageInput data-uri alias: %v", err)
	}
	if !strings.HasPrefix(string(data), "data:image/png;base64,") {
		t.Errorf("data-uri alias output missing prefix: %.40s", data)
	}
}

func TestDecodeImageInput_shortBase64NotImage(t *testing.T) {
	// Valid base64 but decoded result is not an image.
	content := []byte(base64.StdEncoding.EncodeToString([]byte("hello world hello world hello")))
	_, _, err := DecodeImageInput(content, "")
	if err == nil {
		t.Error("DecodeImageInput non-image base64: expected error")
	}
}
