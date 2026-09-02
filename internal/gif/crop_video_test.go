package gif

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDefaultCropOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "mp4 in dir", input: "/tmp/org.mp4", want: "/tmp/org_crop.mp4"},
		{name: "plain name", input: "clip.mov", want: "clip_crop.mov"},
		{name: "no extension", input: "dir/raw", want: "dir/raw_crop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultCropOutput(tt.input); got != tt.want {
				t.Errorf("defaultCropOutput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestCropVideoIntegration 用真实 ffmpeg 验证裁边视频输出存在、原文件保留、且只裁指定边。
func TestCropVideoIntegration(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	if err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=640x360:d=1:r=6",
		"-pix_fmt", "yuv420p", in).Run(); err != nil {
		t.Fatalf("failed to generate test video: %v", err)
	}

	out, err := CropVideo(CropVideoOptions{
		Input:      in,
		CropMargin: CropMargins{Top: 64, Bottom: 64},
		Verbose:    true,
	})
	if err != nil {
		t.Fatalf("CropVideo failed: %v", err)
	}
	if _, statErr := os.Stat(out); statErr != nil {
		t.Fatalf("output video not created: %v", statErr)
	}
	if filepath.Base(out) != "in_crop.mp4" {
		t.Errorf("unexpected output name %q", out)
	}
	// 输出应与输入同目录（不覆盖原文件）。
	if filepath.Dir(out) != filepath.Dir(in) {
		t.Errorf("output dir %q != input dir %q", filepath.Dir(out), filepath.Dir(in))
	}
	if _, statErr := os.Stat(in); statErr != nil {
		t.Errorf("original input should be preserved, got: %v", statErr)
	}
	// 只裁上下：宽度不变、高度减 128。
	if w, h := probeVideoSize(out); w != 640 || h != 360-128 {
		t.Errorf("expected 640x232 output after top/bottom crop, got %dx%d", w, h)
	}
}
