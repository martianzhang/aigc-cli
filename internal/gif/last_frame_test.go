package gif

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDefaultLastFrameOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "mp4 in dir", input: "/tmp/org.mp4", want: "/tmp/org_lastframe.png"},
		{name: "plain name", input: "clip.mov", want: "clip_lastframe.png"},
		{name: "no extension", input: "dir/raw", want: "dir/raw_lastframe.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultLastFrameOutput(tt.input); got != tt.want {
				t.Errorf("defaultLastFrameOutput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractLastFrameEmptyInput(t *testing.T) {
	_, err := ExtractLastFrame(ExtractLastFrameOptions{Input: "  "})
	if err == nil {
		t.Fatal("ExtractLastFrame with empty input: expected error")
	}
}

// TestExtractLastFrameIntegration 用真实 ffmpeg 验证尾帧被抽出为图片、原文件保留。
func TestExtractLastFrameIntegration(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
	if err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:d=1:r=5",
		"-pix_fmt", "yuv420p", in).Run(); err != nil {
		t.Fatalf("failed to generate test video: %v", err)
	}

	out, err := ExtractLastFrame(ExtractLastFrameOptions{Input: in})
	if err != nil {
		t.Fatalf("ExtractLastFrame failed: %v", err)
	}
	if filepath.Base(out) != "in_lastframe.png" {
		t.Errorf("output name = %q, want in_lastframe.png", filepath.Base(out))
	}
	if filepath.Dir(out) != filepath.Dir(in) {
		t.Errorf("output dir %q != input dir %q", filepath.Dir(out), filepath.Dir(in))
	}
	info, statErr := os.Stat(out)
	if statErr != nil || info.Size() == 0 {
		t.Fatalf("output image not created / empty: %v", statErr)
	}
	if _, statErr := os.Stat(in); statErr != nil {
		t.Errorf("original input should be preserved, got: %v", statErr)
	}
}
