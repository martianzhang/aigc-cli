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
