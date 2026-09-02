package gif

import (
	"image"
	_ "image/gif" // 注册 GIF 解码器，供 image.Decode 使用
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestConvertMissingFFmpeg 验证 ffmpeg 缺失时返回带安装提示的错误。
func TestConvertMissingFFmpeg(t *testing.T) {
	if Available() {
		t.Skip("ffmpeg present; missing-ffmpeg path not exercised")
	}
	_, err := Convert(ConvertOptions{Input: "x.mp4"})
	if err == nil {
		t.Fatal("expected error when ffmpeg is missing")
	}
	if !strings.Contains(err.Error(), "brew install ffmpeg") {
		t.Errorf("error should include install hint, got: %v", err)
	}
}

// TestConvertIntegration 用真实 ffmpeg 跑一次最小转换（LookPath 守卫，缺 ffmpeg 跳过）。
func TestConvertIntegration(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	// 用 ffmpeg 自生成一张测试用短视频（纯色，1 秒 6 帧）。
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	if err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:d=1:r=6",
		"-pix_fmt", "yuv420p", in).Run(); err != nil {
		t.Fatalf("failed to generate test video: %v", err)
	}

	out := filepath.Join(dir, "out.gif")
	if _, err := Convert(ConvertOptions{Input: in, Output: out, Width: 32, Verbose: true}); err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if _, statErr := os.Stat(out); statErr != nil {
		t.Fatalf("output gif not created: %v", statErr)
	}
	if filepath.Base(out) != "out.gif" {
		t.Errorf("unexpected output name %q", out)
	}
}

// TestConvertCropMargin 验证 --crop-margin：64x64 视频裁 8px 边后，输出宽高比保持不变。
// 需要 ffprobe 探测源尺寸；ffprobe 缺失时跳过（退化路径不校验比例）。
func TestConvertCropMargin(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not on PATH; ratio-preserving path not exercised")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	if err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:d=1:r=6",
		"-pix_fmt", "yuv420p", in).Run(); err != nil {
		t.Fatalf("failed to generate test video: %v", err)
	}

	out := filepath.Join(dir, "crop.gif")
	if _, err := Convert(ConvertOptions{Input: in, Output: out, Width: 32, CropMargin: CropMargins{Top: 8, Right: 8, Bottom: 8, Left: 8}}); err != nil {
		t.Fatalf("Convert with crop-margin failed: %v", err)
	}
	gifFile, gifErr := os.Open(out)
	if gifErr != nil {
		t.Fatalf("open output gif: %v", gifErr)
	}
	defer gifFile.Close()
	img, _, decErr := image.Decode(gifFile)
	if decErr != nil {
		t.Fatalf("decode output gif: %v", decErr)
	}
	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Errorf("expected 32x32 output, got %dx%d", b.Dx(), b.Dy())
	}
}

// TestConvertCropMarginTooLarge 验证 crop-margin 超过视频一半尺寸时报错。
func TestConvertCropMarginTooLarge(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	if err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:d=1:r=6",
		"-pix_fmt", "yuv420p", in).Run(); err != nil {
		t.Fatalf("failed to generate test video: %v", err)
	}
	_, err := Convert(ConvertOptions{Input: in, Output: filepath.Join(dir, "too-large.gif"), CropMargin: CropMargins{Top: 100, Right: 100, Bottom: 100, Left: 100}})
	if err == nil {
		t.Fatal("expected error for oversized crop-margin")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error should mention oversized margin, got: %v", err)
	}
}
