package annotate

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadImage 验证 LoadImage 解码图片文件。
func TestLoadImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "img.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 25), G: uint8(y * 25), B: 100, A: 255})
		}
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, err := LoadImage(path)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	if got.Bounds().Dx() != 10 || got.Bounds().Dy() != 10 {
		t.Fatalf("bounds = %v, want 10x10", got.Bounds())
	}

	if _, err := LoadImage(filepath.Join(t.TempDir(), "missing.png")); err == nil {
		t.Fatal("LoadImage on missing file should error")
	}
}

// TestAnnotateVideoCallbackSkeletonMissing 验证模型缺失时返回明确错误。
func TestAnnotateVideoCallbackSkeletonMissing(t *testing.T) {
	// 用不存在的 HOME 强制 ModelsDir 指向空目录。
	t.Setenv("HOME", filepath.Join(t.TempDir(), "nohome"))
	_, _, err := AnnotateVideoCallback(Options{Skeleton: true})
	if err == nil {
		t.Fatal("expected error when models missing")
	}
	// 报错应指向 onnx runtime 或 skeleton 模型（取决于检查顺序）。
	if !strings.Contains(err.Error(), "onnx runtime library not found") &&
		!strings.Contains(err.Error(), "skeleton model not found") {
		t.Fatalf("error = %v, want missing onnx runtime or skeleton model hint", err)
	}
}

// TestModelsDir 验证默认模型目录路径。
func TestModelsDir(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	if got := ModelsDir(); got != "/tmp/fakehome/.config/aigc-cli/models" {
		t.Fatalf("ModelsDir = %q, want /tmp/fakehome/.config/aigc-cli/models", got)
	}
}

// TestModelsDirHomeUnset 验证 HOME 未设置时回退到相对路径。
func TestModelsDirHomeUnset(t *testing.T) {
	t.Setenv("HOME", "")
	want := filepath.Join(".config", "aigc-cli", "models")
	if got := ModelsDir(); got != want {
		t.Fatalf("ModelsDir = %q, want %q", got, want)
	}
}

// TestToRGBAIdentity 验证 *image.RGBA 输入原样返回同一指针。
func TestToRGBAIdentity(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	src.Set(1, 2, color.RGBA{R: 10, G: 20, B: 30, A: 255})

	if got := toRGBA(src); got != src {
		t.Fatalf("toRGBA(*image.RGBA) = %p, want same pointer %p", got, src)
	}
}

// TestToRGBAFromGray 验证 *image.Gray 转换为新的 *image.RGBA 且边界与像素一致。
func TestToRGBAFromGray(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 3, 2))
	src.SetGray(0, 0, color.Gray{Y: 0})
	src.SetGray(1, 0, color.Gray{Y: 128})
	src.SetGray(2, 1, color.Gray{Y: 255})

	got := toRGBA(src)
	if got == nil {
		t.Fatal("toRGBA returned nil")
	}
	assertSamePixels(t, got, src)
}

// TestToRGBAFromNRGBA 验证 *image.NRGBA 转换为新的 *image.RGBA 且边界与像素一致。
func TestToRGBAFromNRGBA(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	src.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 128})
	src.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 0})

	got := toRGBA(src)
	if got == nil {
		t.Fatal("toRGBA returned nil")
	}
	assertSamePixels(t, got, src)
}

// assertSamePixels 验证 got 与 src 边界一致且逐像素等值（统一按 RGBA 比较）。
func assertSamePixels(t *testing.T, got *image.RGBA, src image.Image) {
	t.Helper()
	if got.Bounds() != src.Bounds() {
		t.Fatalf("bounds = %v, want %v", got.Bounds(), src.Bounds())
	}
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			want := color.RGBAModel.Convert(src.At(x, y)).(color.RGBA)
			if have := got.At(x, y).(color.RGBA); have != want {
				t.Errorf("pixel (%d,%d) = %v, want %v", x, y, have, want)
			}
		}
	}
}

// TestDefaultLibPathMissing 验证目录下无库文件时返回错误。
func TestDefaultLibPathMissing(t *testing.T) {
	dir := t.TempDir()
	got, err := defaultLibPath(dir)
	if err == nil {
		t.Fatalf("defaultLibPath(%q) = %q, want error", dir, got)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("error = %v, want mention of %q", err, dir)
	}
}

// TestDefaultLibPathDylib 验证 libonnxruntime.dylib 存在时返回其路径。
func TestDefaultLibPathDylib(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "libonnxruntime.dylib")
	if err := os.WriteFile(want, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := defaultLibPath(dir)
	if err != nil {
		t.Fatalf("defaultLibPath: %v", err)
	}
	if got != want {
		t.Fatalf("defaultLibPath = %q, want %q", got, want)
	}
}

// TestDefaultLibPathSoFallback 验证仅有 libonnxruntime.so 时（darwin 回退）返回其路径。
func TestDefaultLibPathSoFallback(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "libonnxruntime.so")
	if err := os.WriteFile(want, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := defaultLibPath(dir)
	if err != nil {
		t.Fatalf("defaultLibPath: %v", err)
	}
	if got != want {
		t.Fatalf("defaultLibPath = %q, want %q", got, want)
	}
}

// TestAnnotatorsCloseEmpty 验证 close 函数切片为空时 Close 不 panic。
func TestAnnotatorsCloseEmpty(t *testing.T) {
	a := &annotators{}
	a.Close()
}
