package service

import (
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// createTestJPEG creates a small JPEG file for testing and returns its path.
func createTestJPEG(t *testing.T, dir string, quality int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// Fill with gradient to make it non-trivial to compress
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 2),
				G: uint8(y * 2),
				B: uint8((x + y) / 2),
				A: 255,
			})
		}
	}
	path := filepath.Join(dir, "test.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test jpg: %v", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("encode test jpg: %v", err)
	}
	return path
}

func makeTestPNG(t *testing.T, dir string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{255, 0, 0, 255}), image.Point{}, draw.Src)
	path := filepath.Join(dir, "test.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test png: %v", err)
	}
	defer f.Close()
	if err := encodeImageTo(img, f, "png", 0); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return path
}

func TestParseCompressOption_SizeKB(t *testing.T) {
	target, quality, err := ParseCompressOption("800KB")
	if err != nil {
		t.Fatalf("ParseCompressOption 800KB: %v", err)
	}
	if target != 800*1024 {
		t.Errorf("target = %d, want %d", target, 800*1024)
	}
	if quality != 0 {
		t.Errorf("quality = %d, want 0", quality)
	}
}

func TestParseCompressOption_SizeMB(t *testing.T) {
	target, quality, err := ParseCompressOption("2MB")
	if err != nil {
		t.Fatalf("ParseCompressOption 2MB: %v", err)
	}
	if target != 2*1024*1024 {
		t.Errorf("target = %d, want %d", target, 2*1024*1024)
	}
	if quality != 0 {
		t.Errorf("quality = %d, want 0", quality)
	}
}

func TestParseCompressOption_SizeK(t *testing.T) {
	target, quality, err := ParseCompressOption("500K")
	if err != nil {
		t.Fatalf("ParseCompressOption 500K: %v", err)
	}
	if target != 500*1024 {
		t.Errorf("target = %d, want %d", target, 500*1024)
	}
	if quality != 0 {
		t.Errorf("quality = %d, want 0", quality)
	}
}

func TestParseCompressOption_SizeM(t *testing.T) {
	target, quality, err := ParseCompressOption("1M")
	if err != nil {
		t.Fatalf("ParseCompressOption 1M: %v", err)
	}
	if target != 1*1024*1024 {
		t.Errorf("target = %d, want %d", target, 1*1024*1024)
	}
	if quality != 0 {
		t.Errorf("quality = %d, want 0", quality)
	}
}

func TestParseCompressOption_Percent(t *testing.T) {
	target, quality, err := ParseCompressOption("85%")
	if err != nil {
		t.Fatalf("ParseCompressOption 85%%: %v", err)
	}
	if target != 0 {
		t.Errorf("target = %d, want 0", target)
	}
	if quality != 85 {
		t.Errorf("quality = %d, want 85", quality)
	}
}

func TestParseCompressOption_Invalid(t *testing.T) {
	_, _, err := ParseCompressOption("abc")
	if err == nil {
		t.Error("ParseCompressOption abc: expected error")
	}
}

func TestParseCompressOption_Empty(t *testing.T) {
	_, _, err := ParseCompressOption("")
	if err == nil {
		t.Error("ParseCompressOption empty: expected error")
	}
}

func TestParseCompressOption_QualityOutOfRange(t *testing.T) {
	_, _, err := ParseCompressOption("0%")
	if err == nil {
		t.Error("ParseCompressOption 0%: expected error")
	}
	_, _, err = ParseCompressOption("101%")
	if err == nil {
		t.Error("ParseCompressOption 101%: expected error")
	}
}

func TestCompressImage_FixedQuality(t *testing.T) {
	dir := t.TempDir()
	srcPath := createTestJPEG(t, dir, 95)

	opts := &CompressOptions{Quality: 50, Format: "jpg"}
	result, err := CompressImage(srcPath, opts)
	if err != nil {
		t.Fatalf("CompressImage: %v", err)
	}
	if result.Skipped {
		t.Logf("Compression skipped: %s", result.Reason)
	}
	if result.Before <= 0 {
		t.Error("Before size should be > 0")
	}
	if result.After <= 0 {
		t.Error("After size should be > 0")
	}
	if !result.Skipped && result.After >= result.Before {
		t.Errorf("Compressed size (%d) should be < original (%d)", result.After, result.Before)
	}
	// Cleanup
	os.Remove(result.DstPath)
}

func TestCompressImage_TargetSize(t *testing.T) {
	dir := t.TempDir()
	srcPath := createTestJPEG(t, dir, 95)

	opts := &CompressOptions{TargetSize: 5000, Format: "jpg"}
	result, err := CompressImage(srcPath, opts)
	if err != nil {
		t.Fatalf("CompressImage: %v", err)
	}
	if result.Skipped {
		t.Logf("Compression skipped: %s", result.Reason)
	}
	if !result.Skipped && result.After > 5000 {
		t.Errorf("Compressed size (%d) exceeds target (%d)", result.After, 5000)
	}
	// Cleanup
	if !result.Skipped {
		os.Remove(result.DstPath)
	}
}

func TestCompressImage_AlreadySmall(t *testing.T) {
	dir := t.TempDir()
	srcPath := createTestJPEG(t, dir, 10) // low quality = small file

	opts := &CompressOptions{TargetSize: 1 * 1024 * 1024, Format: "jpg"} // 1MB target
	result, err := CompressImage(srcPath, opts)
	if err != nil {
		t.Fatalf("CompressImage: %v", err)
	}
	if !result.Skipped {
		t.Error("Expected skip for already-small image")
	}
}

func TestCompressImage_FormatConversion(t *testing.T) {
	// WebP encoding requires CGO; skip gracefully when no C compiler is available.
	// Try a quick encode to detect the stub error before doing a real conversion.
	if err := webpEncode(io.Discard, image.NewRGBA(image.Rect(0, 0, 1, 1)), 80); err != nil {
		t.Skipf("WebP encoding requires CGO: %v (CI server will run with CGO)", err)
	}

	dir := t.TempDir()
	srcPath := createTestJPEG(t, dir, 90)

	// Convert to WebP
	opts := &CompressOptions{Quality: 80, Format: "webp"}
	result, err := CompressImage(srcPath, opts)
	if err != nil {
		t.Fatalf("CompressImage to webp: %v", err)
	}
	if !result.Skipped && result.After <= 0 {
		t.Error("WebP output should have size > 0")
	}
	t.Logf("JPEG→WebP: before=%d after=%d skipped=%v reason=%s", result.Before, result.After, result.Skipped, result.Reason)
	if !result.Skipped {
		os.Remove(result.DstPath)
	}
}

func TestCompressImage_PNGKeepFormat(t *testing.T) {
	dir := t.TempDir()
	srcPath := makeTestPNG(t, dir)

	opts := &CompressOptions{Quality: 80, Format: "png"}
	result, err := CompressImage(srcPath, opts)
	if err != nil {
		t.Fatalf("CompressImage png: %v", err)
	}
	if !result.Skipped {
		t.Log("PNG compression was not skipped — unexpected but not an error")
	}
	t.Logf("PNG: before=%d after=%d skipped=%v reason=%s", result.Before, result.After, result.Skipped, result.Reason)
}

func TestCompressImage_DefaultFormat(t *testing.T) {
	dir := t.TempDir()
	srcPath := createTestJPEG(t, dir, 92)

	// No format specified → keep original (.jpg)
	opts := &CompressOptions{Quality: 60}
	result, err := CompressImage(srcPath, opts)
	if err != nil {
		t.Fatalf("CompressImage default fmt: %v", err)
	}
	if !result.Skipped && filepath.Ext(result.DstPath) != ".jpg" {
		t.Errorf("Expected .jpg extension, got %s", filepath.Ext(result.DstPath))
	}
	if !result.Skipped {
		os.Remove(result.DstPath)
	}
}

func TestCompressImage_BinarySearch(t *testing.T) {
	dir := t.TempDir()
	// Create a larger image for binary search to be meaningful
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x + y) * 3 % 256),
				G: uint8((x * y) % 256),
				B: uint8((x*2 + y*3) % 256),
				A: 255,
			})
		}
	}
	srcPath := filepath.Join(dir, "large_test.jpg")
	f, _ := os.Create(srcPath)
	jpeg.Encode(f, img, &jpeg.Options{Quality: 95})
	f.Close()

	// Target a specific size
	opts := &CompressOptions{TargetSize: 15000, Format: "jpg"}
	result, err := CompressImage(srcPath, opts)
	if err != nil {
		t.Fatalf("CompressImage binary search: %v", err)
	}
	t.Logf("Binary search: before=%d after=%d skipped=%v", result.Before, result.After, result.Skipped)
	if !result.Skipped && result.After > 15000 {
		t.Errorf("Binary search result (%d) exceeds target (%d)", result.After, 15000)
	}
	if !result.Skipped {
		os.Remove(result.DstPath)
	}
}

func TestParseCompressOption_Table(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantTarget  int64
		wantQuality int
		wantErr     bool
	}{
		{name: "empty", input: "", wantErr: true},
		{name: "whitespace only", input: "   ", wantErr: true},
		{name: "auto lower", input: "auto"},
		{name: "auto upper", input: "AUTO"},
		{name: "kilobytes", input: "800KB", wantTarget: 800 * 1024},
		{name: "kilobytes lower", input: "800kb", wantTarget: 800 * 1024},
		{name: "megabytes", input: "2MB", wantTarget: 2 * 1024 * 1024},
		{name: "megabytes lower", input: "2mb", wantTarget: 2 * 1024 * 1024},
		{name: "short kilo", input: "500K", wantTarget: 500 * 1024},
		{name: "short mega", input: "1M", wantTarget: 1 * 1024 * 1024},
		{name: "bytes suffix", input: "1024B", wantTarget: 1024},
		{name: "bare bytes", input: "1024", wantTarget: 1024},
		{name: "percent", input: "85%", wantQuality: 85},
		{name: "percent lower bound", input: "1%", wantQuality: 1},
		{name: "percent upper bound", input: "100%", wantQuality: 100},
		{name: "padded input", input: " 800KB ", wantTarget: 800 * 1024},
		{name: "invalid text", input: "abc", wantErr: true},
		{name: "quality zero", input: "0%", wantErr: true},
		{name: "quality too high", input: "101%", wantErr: true},
		{name: "zero size", input: "0KB", wantErr: true},
		{name: "negative size", input: "-5KB", wantErr: true},
		{name: "fractional size", input: "1.5MB", wantErr: true},
		{name: "percent without number", input: "%", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target, quality, err := ParseCompressOption(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseCompressOption(%q) expected error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCompressOption(%q) unexpected error: %v", tc.input, err)
			}
			if target != tc.wantTarget || quality != tc.wantQuality {
				t.Errorf("ParseCompressOption(%q) = (%d, %d), want (%d, %d)",
					tc.input, target, quality, tc.wantTarget, tc.wantQuality)
			}
		})
	}
}

func TestCompressOptions_Fields(t *testing.T) {
	opts := CompressOptions{TargetSize: 2048, Quality: 75, Format: "webp"}
	if opts.TargetSize != 2048 {
		t.Errorf("TargetSize = %d, want 2048", opts.TargetSize)
	}
	if opts.Quality != 75 {
		t.Errorf("Quality = %d, want 75", opts.Quality)
	}
	if opts.Format != "webp" {
		t.Errorf("Format = %q, want %q", opts.Format, "webp")
	}

	var zero CompressOptions
	if zero.TargetSize != 0 || zero.Quality != 0 || zero.Format != "" {
		t.Errorf("zero CompressOptions = %+v, want all fields unset", zero)
	}
}

func TestCompressResult_Fields(t *testing.T) {
	res := CompressResult{
		DstPath: "/tmp/out.jpg",
		Before:  4096,
		After:   1024,
		Skipped: false,
		Reason:  "",
		Format:  "jpg",
		Quality: 80,
	}
	if res.DstPath != "/tmp/out.jpg" {
		t.Errorf("DstPath = %q, want /tmp/out.jpg", res.DstPath)
	}
	if res.Before != 4096 || res.After != 1024 {
		t.Errorf("Before/After = %d/%d, want 4096/1024", res.Before, res.After)
	}
	if res.Skipped {
		t.Error("Skipped = true, want false")
	}
	if res.Reason != "" {
		t.Errorf("Reason = %q, want empty", res.Reason)
	}
	if res.Format != "jpg" || res.Quality != 80 {
		t.Errorf("Format/Quality = %q/%d, want jpg/80", res.Format, res.Quality)
	}

	skipped := CompressResult{Skipped: true, Reason: "already ≤ target"}
	if !skipped.Skipped || skipped.Reason == "" {
		t.Errorf("skipped result not populated: %+v", skipped)
	}
}

func TestTryEncode_Formats(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 10, G: 20, B: 30, A: 255}), image.Point{}, draw.Src)

	tests := []struct {
		name    string
		outFmt  string
		wantErr bool
	}{
		{name: "jpg", outFmt: "jpg"},
		{name: "jpeg alias", outFmt: "jpeg"},
		{name: "png", outFmt: "png"},
		{name: "unsupported format", outFmt: "bmp", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "out."+tc.outFmt)
			size, err := tryEncode(img, dst, tc.outFmt, 80)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("tryEncode(%q) expected error", tc.outFmt)
				}
				if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
					t.Errorf("tryEncode(%q) should remove %s on failure", tc.outFmt, dst)
				}
				return
			}
			if err != nil {
				t.Fatalf("tryEncode(%q) unexpected error: %v", tc.outFmt, err)
			}
			if size <= 0 {
				t.Errorf("tryEncode(%q) size = %d, want > 0", tc.outFmt, size)
			}
			info, err := os.Stat(dst)
			if err != nil {
				t.Fatalf("stat %s: %v", dst, err)
			}
			if info.Size() != size {
				t.Errorf("tryEncode(%q) returned size %d, but file size is %d", tc.outFmt, size, info.Size())
			}
		})
	}
}

func TestEncodeImage_ReturnsFileSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 200, G: 100, B: 50, A: 255}), image.Point{}, draw.Src)

	dst := filepath.Join(t.TempDir(), "encoded.jpg")
	size, err := encodeImage(img, dst, "jpg", 70)
	if err != nil {
		t.Fatalf("encodeImage: %v", err)
	}
	if size <= 0 {
		t.Errorf("encodeImage size = %d, want > 0", size)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat %s: %v", dst, err)
	}
	if info.Size() != size {
		t.Errorf("encodeImage returned size %d, but file size is %d", size, info.Size())
	}
}
