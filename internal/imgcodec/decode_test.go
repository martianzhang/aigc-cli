package imgcodec

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"testing"
)

// panicMagic is a magic prefix registered only by this test, so the panicking
// decoder below is exercised without colliding with any real codec.
//
// It stands in for the wasm decoders (jpegli/webp/avif/jpegxl), which trap with
// a Go panic instead of returning an error on malformed input.
const panicMagic = "\x00aigc-panic-test\x00"

func init() {
	boom := func(io.Reader) (image.Image, error) { panic("wasm trap: unreachable") }
	boomConfig := func(io.Reader) (image.Config, error) { panic("wasm trap: unreachable") }
	image.RegisterFormat("aigc-panic-test", panicMagic, boom, boomConfig)
}

// TestDecodeRecoversPanic verifies a panicking decoder is converted to an error
// rather than crashing the process.
func TestDecodeRecoversPanic(t *testing.T) {
	if _, _, err := Decode(bytes.NewReader([]byte(panicMagic))); err == nil {
		t.Fatal("Decode() = nil error, want the recovered panic as an error")
	}
}

// TestDecodeConfigRecoversPanic is the DecodeConfig counterpart: this is the
// exact path that crashed `ideas --preview` on a malformed remote JPEG.
func TestDecodeConfigRecoversPanic(t *testing.T) {
	if _, _, err := DecodeConfig(bytes.NewReader([]byte(panicMagic))); err == nil {
		t.Fatal("DecodeConfig() = nil error, want the recovered panic as an error")
	}
}

// TestDecodeConfigValidImage guards against the wrappers breaking normal decoding.
func TestDecodeConfigValidImage(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	cfg, format, err := DecodeConfig(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("DecodeConfig() unexpected error: %v", err)
	}
	if format != "png" || cfg.Width != 3 || cfg.Height != 2 {
		t.Fatalf("DecodeConfig() = (%s, %dx%d), want (png, 3x2)", format, cfg.Width, cfg.Height)
	}
}

func pngHeader(width, height uint32) []byte {
	chunk := make([]byte, 0, 17)
	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:4], width)
	binary.BigEndian.PutUint32(data[4:8], height)
	data[8] = 8 // bit depth
	data[9] = 2 // color type: RGB
	chunk = append(chunk, 0, 0, 0, 13)
	chunk = append(chunk, []byte("IHDR")...)
	chunk = append(chunk, data...)
	crc := crc32.ChecksumIEEE(append([]byte("IHDR"), data...))
	chunk = binary.BigEndian.AppendUint32(chunk, crc)
	return append([]byte("\x89PNG\r\n\x1a\n"), chunk...)
}

func TestDecodeRejectsTooManyPixels(t *testing.T) {
	header := pngHeader(20_000, 20_000) // 400 MP > 100 MP limit
	if _, _, err := Decode(bytes.NewReader(header)); err == nil {
		t.Fatal("Decode() = nil error, want image-too-large")
	}
}
