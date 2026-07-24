package service

import (
	"encoding/binary"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

// createTestJPG builds and writes a small JPEG at path for testing.
func createTestJPG(t *testing.T, path string, w, h int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create test JPEG: %v", err)
	}
	defer f.Close()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("failed to encode test JPEG: %v", err)
	}
}

// newTestIfdBuilder creates a boilerplate EXIF IfdBuilder for testing.
func newTestIfdBuilder(t *testing.T) *exif.IfdBuilder {
	t.Helper()
	ti := exif.NewTagIndex()
	ifdMapping := exifcommon.NewIfdMapping()
	if err := exifcommon.LoadStandardIfds(ifdMapping); err != nil {
		t.Fatalf("load IFD mapping: %v", err)
	}
	return exif.NewIfdBuilder(ifdMapping, ti, exifcommon.IfdStandardIfdIdentity, binary.BigEndian)
}

// ── ReadDescription ──

func TestReadDescription_pngWithDescription(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")
	createTestPNG(t, path, 10, 10)

	want := "a test description"
	if err := WriteDescription(path, want); err != nil {
		t.Fatalf("WriteDescription failed: %v", err)
	}

	got, err := ReadDescription(path)
	if err != nil {
		t.Fatalf("ReadDescription failed: %v", err)
	}
	if got != want {
		t.Errorf("ReadDescription() = %q, want %q", got, want)
	}
}

func TestReadDescription_pngNoDescription(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")
	createTestPNG(t, path, 10, 10)

	got, err := ReadDescription(path)
	if err != nil {
		t.Fatalf("ReadDescription failed: %v", err)
	}
	if got != "" {
		t.Errorf("ReadDescription() = %q, want empty string", got)
	}
}

func TestReadDescription_jpegWithDescription(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jpg")
	createTestJPG(t, path, 10, 10)

	want := "jpeg description"
	if err := WriteDescription(path, want); err != nil {
		t.Fatalf("WriteDescription failed: %v", err)
	}

	got, err := ReadDescription(path)
	if err != nil {
		t.Fatalf("ReadDescription failed: %v", err)
	}
	if got != want {
		t.Errorf("ReadDescription() = %q, want %q", got, want)
	}
}

func TestReadDescription_nonexistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.png")
	_, err := ReadDescription(path)
	if err == nil {
		t.Error("ReadDescription on nonexistent file should return error")
	}
}

func TestReadDescription_unsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.gif")
	if err := os.WriteFile(path, []byte("GIF89a"), 0644); err != nil {
		t.Fatalf("failed to create dummy gif: %v", err)
	}

	got, err := ReadDescription(path)
	if err != nil {
		t.Fatalf("ReadDescription failed: %v", err)
	}
	if got != "" {
		t.Errorf("ReadDescription() = %q, want empty string for unsupported format", got)
	}
}

// ── setDesc ──

func TestSetDesc_writesDescription(t *testing.T) {
	ib := newTestIfdBuilder(t)
	if err := setDesc(ib, "hello world"); err != nil {
		t.Fatalf("setDesc failed: %v", err)
	}

	bt, err := ib.FindTagWithName("ImageDescription")
	if err != nil {
		t.Fatalf("FindTagWithName failed: %v", err)
	}

	val := bt.Value()
	if !val.IsBytes() {
		t.Fatal("expected byte value")
	}
	if string(val.Bytes()) != "hello world" {
		t.Errorf("value = %q, want %q", string(val.Bytes()), "hello world")
	}
}

func TestSetDesc_overwritesExisting(t *testing.T) {
	ib := newTestIfdBuilder(t)
	if err := setDesc(ib, "first"); err != nil {
		t.Fatalf("setDesc first failed: %v", err)
	}
	if err := setDesc(ib, "second"); err != nil {
		t.Fatalf("setDesc second failed: %v", err)
	}

	bt, err := ib.FindTagWithName("ImageDescription")
	if err != nil {
		t.Fatalf("FindTagWithName failed: %v", err)
	}

	val := bt.Value()
	if string(val.Bytes()) != "second" {
		t.Errorf("value = %q, want %q", string(val.Bytes()), "second")
	}
}

func TestSetDesc_emptyDescription(t *testing.T) {
	ib := newTestIfdBuilder(t)
	if err := setDesc(ib, ""); err != nil {
		t.Fatalf("setDesc failed: %v", err)
	}

	bt, err := ib.FindTagWithName("ImageDescription")
	if err != nil {
		t.Fatalf("FindTagWithName failed: %v", err)
	}

	val := bt.Value()
	if string(val.Bytes()) != "" {
		t.Errorf("value = %q, want empty string", string(val.Bytes()))
	}
}

// ── WriteDescription ──

func TestWriteDescription_png(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")
	createTestPNG(t, path, 10, 10)

	want := "png write test"
	if err := WriteDescription(path, want); err != nil {
		t.Fatalf("WriteDescription failed: %v", err)
	}

	got, err := ReadDescription(path)
	if err != nil {
		t.Fatalf("ReadDescription failed: %v", err)
	}
	if got != want {
		t.Errorf("ReadDescription() = %q, want %q", got, want)
	}
}

func TestWriteDescription_pngOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")
	createTestPNG(t, path, 10, 10)

	if err := WriteDescription(path, "original"); err != nil {
		t.Fatalf("WriteDescription first failed: %v", err)
	}
	if err := WriteDescription(path, "updated"); err != nil {
		t.Fatalf("WriteDescription second failed: %v", err)
	}

	got, err := ReadDescription(path)
	if err != nil {
		t.Fatalf("ReadDescription failed: %v", err)
	}
	if got != "updated" {
		t.Errorf("ReadDescription() = %q, want %q", got, "updated")
	}
}

func TestWriteDescription_jpeg(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jpg")
	createTestJPG(t, path, 10, 10)

	want := "jpeg write test"
	if err := WriteDescription(path, want); err != nil {
		t.Fatalf("WriteDescription failed: %v", err)
	}

	got, err := ReadDescription(path)
	if err != nil {
		t.Fatalf("ReadDescription failed: %v", err)
	}
	if got != want {
		t.Errorf("ReadDescription() = %q, want %q", got, want)
	}
}

func TestWriteDescription_jpegOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jpg")
	createTestJPG(t, path, 10, 10)

	if err := WriteDescription(path, "original"); err != nil {
		t.Fatalf("WriteDescription first failed: %v", err)
	}
	if err := WriteDescription(path, "updated"); err != nil {
		t.Fatalf("WriteDescription second failed: %v", err)
	}

	got, err := ReadDescription(path)
	if err != nil {
		t.Fatalf("ReadDescription failed: %v", err)
	}
	if got != "updated" {
		t.Errorf("ReadDescription() = %q, want %q", got, "updated")
	}
}

func TestWriteDescription_nonexistentParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "test.png")
	if err := WriteDescription(path, "desc"); err == nil {
		t.Error("WriteDescription on nonexistent parent dir should return error")
	}
}

func TestWriteDescription_emptyDescription(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")
	createTestPNG(t, path, 10, 10)

	if err := WriteDescription(path, "before"); err != nil {
		t.Fatalf("WriteDescription first failed: %v", err)
	}
	if err := WriteDescription(path, ""); err != nil {
		t.Fatalf("WriteDescription empty failed: %v", err)
	}

	got, err := ReadDescription(path)
	if err != nil {
		t.Fatalf("ReadDescription failed: %v", err)
	}
	if got != "" {
		t.Errorf("ReadDescription() = %q, want empty string", got)
	}
}

func TestWriteDescription_unsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.gif")
	if err := os.WriteFile(path, []byte("GIF89a"), 0644); err != nil {
		t.Fatalf("failed to create dummy gif: %v", err)
	}

	if err := WriteDescription(path, "desc"); err == nil {
		t.Error("WriteDescription on unsupported format should return error")
	}
}

// ── writePngDescription ──

func TestWritePngDescription_newFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")
	createTestPNG(t, path, 10, 10)

	want := "png desc"
	if err := writePngDescription(path, want); err != nil {
		t.Fatalf("writePngDescription failed: %v", err)
	}

	got, err := readPngDescription(path)
	if err != nil {
		t.Fatalf("readPngDescription failed: %v", err)
	}
	if got != want {
		t.Errorf("readPngDescription() = %q, want %q", got, want)
	}
}

func TestWritePngDescription_overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")
	createTestPNG(t, path, 10, 10)

	if err := writePngDescription(path, "first"); err != nil {
		t.Fatalf("writePngDescription first failed: %v", err)
	}
	if err := writePngDescription(path, "second"); err != nil {
		t.Fatalf("writePngDescription second failed: %v", err)
	}

	got, err := readPngDescription(path)
	if err != nil {
		t.Fatalf("readPngDescription failed: %v", err)
	}
	if got != "second" {
		t.Errorf("readPngDescription() = %q, want %q", got, "second")
	}
}

func TestWritePngDescription_nonexistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.png")
	if err := writePngDescription(path, "desc"); err == nil {
		t.Error("writePngDescription on nonexistent file should return error")
	}
}

// ── writeJpegDescription ──

func TestWriteJpegDescription_newFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jpg")
	createTestJPG(t, path, 10, 10)

	want := "jpeg desc"
	if err := writeJpegDescription(path, want); err != nil {
		t.Fatalf("writeJpegDescription failed: %v", err)
	}

	got, err := readJpegDescription(path)
	if err != nil {
		t.Fatalf("readJpegDescription failed: %v", err)
	}
	if got != want {
		t.Errorf("readJpegDescription() = %q, want %q", got, want)
	}
}

func TestWriteJpegDescription_overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jpg")
	createTestJPG(t, path, 10, 10)

	if err := writeJpegDescription(path, "first"); err != nil {
		t.Fatalf("writeJpegDescription first failed: %v", err)
	}
	if err := writeJpegDescription(path, "second"); err != nil {
		t.Fatalf("writeJpegDescription second failed: %v", err)
	}

	got, err := readJpegDescription(path)
	if err != nil {
		t.Fatalf("readJpegDescription failed: %v", err)
	}
	if got != "second" {
		t.Errorf("readJpegDescription() = %q, want %q", got, "second")
	}
}

func TestWriteJpegDescription_nonexistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.jpg")
	if err := writeJpegDescription(path, "desc"); err == nil {
		t.Error("writeJpegDescription on nonexistent file should return error")
	}
}

// ── readJpegDescription ──

func TestReadJpegDescription_noExif(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.jpg")
	createTestJPG(t, path, 10, 10)

	got, err := readJpegDescription(path)
	if err != nil {
		t.Fatalf("readJpegDescription failed: %v", err)
	}
	if got != "" {
		t.Errorf("readJpegDescription() = %q, want empty string", got)
	}
}

// ── readPngDescription ──

func TestReadPngDescription_noExif(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.png")
	createTestPNG(t, path, 10, 10)

	got, err := readPngDescription(path)
	if err != nil {
		t.Fatalf("readPngDescription failed: %v", err)
	}
	if got != "" {
		t.Errorf("readPngDescription() = %q, want empty string", got)
	}
}
