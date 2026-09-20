package mcp

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeImageGuarded_validPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ok.png")
	if err := writeSolidPNG(path, 8, 8); err != nil {
		t.Fatalf("write png: %v", err)
	}

	img, format, err := decodeImageGuarded(path)
	if err != nil {
		t.Fatalf("decodeImageGuarded: %v", err)
	}
	if format != "png" {
		t.Errorf("format = %q, want png", format)
	}
	if b := img.Bounds(); b.Dx() != 8 || b.Dy() != 8 {
		t.Errorf("bounds = %v, want 8x8", b)
	}
}

func TestDecodeImageGuarded_oversizedHeaderRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bomb.png")
	if err := os.WriteFile(path, pngWithHeaderDimensions(30000, 30000), 0o644); err != nil {
		t.Fatalf("write bomb: %v", err)
	}

	_, _, err := decodeImageGuarded(path)
	if err == nil {
		t.Fatal("expected decode bomb header to be rejected")
	}
	if !strings.Contains(err.Error(), "image too large: 30000x30000") {
		t.Errorf("error = %v, want image too large", err)
	}
}

func TestDecodeImageGuarded_missingFile(t *testing.T) {
	if _, _, err := decodeImageGuarded(filepath.Join(t.TempDir(), "missing.png")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

// pngWithHeaderDimensions hand-crafts a PNG whose IHDR claims w x h while the
// file carries no pixel data — enough for image.DecodeConfig, which only reads
// the header, and exactly the decode-bomb shape the guard must reject.
func pngWithHeaderDimensions(w, h uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte("\x89PNG\r\n\x1a\n"))

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8] = 8
	ihdr[9] = 6
	writePNGChunk(&buf, "IHDR", ihdr)
	writePNGChunk(&buf, "IEND", nil)
	return buf.Bytes()
}

func writePNGChunk(buf *bytes.Buffer, typ string, data []byte) {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(data)))
	buf.Write(length[:])

	payload := append([]byte(typ), data...)
	buf.Write(payload)

	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc32.ChecksumIEEE(payload))
	buf.Write(sum[:])
}
