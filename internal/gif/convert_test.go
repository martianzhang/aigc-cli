package gif

import (
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

	out, err := Convert(ConvertOptions{Input: in, Width: 32, Verbose: true})
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}
	if _, statErr := os.Stat(out); statErr != nil {
		t.Fatalf("output gif not created: %v", statErr)
	}
	if filepath.Base(out) != "in_32px.gif" {
		t.Errorf("unexpected output name %q", out)
	}
}
