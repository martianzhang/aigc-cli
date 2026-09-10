package gif

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultMP4Output(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "gif in dir", input: "/tmp/anim.gif", want: "/tmp/anim.mp4"},
		{name: "mov in dir", input: "/tmp/clip.mov", want: "/tmp/clip.mp4"},
		{name: "plain name", input: "clip.webm", want: "clip.mp4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultMP4Output(tt.input); got != tt.want {
				t.Errorf("defaultMP4Output(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildMP4Filter(t *testing.T) {
	tests := []struct {
		name string
		crop CropMargins
		want string
	}{
		{
			name: "no crop",
			want: "scale=trunc(iw/2)*2:trunc(ih/2)*2:flags=lanczos",
		},
		{
			name: "crop top bottom",
			crop: CropMargins{Top: 8, Bottom: 8},
			want: "crop=iw-0:ih-16:0:8,scale=trunc(iw/2)*2:trunc(ih/2)*2:flags=lanczos",
		},
		{
			name: "crop left right",
			crop: CropMargins{Left: 4, Right: 4},
			want: "crop=iw-8:ih-0:4:0,scale=trunc(iw/2)*2:trunc(ih/2)*2:flags=lanczos",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildMP4Filter(tt.crop); got != tt.want {
				t.Errorf("buildMP4Filter(%v) = %q, want %q", tt.crop, got, tt.want)
			}
		})
	}
}

// TestToMP4RejectsMP4Input 验证把 mp4 转成自身时会报错（不覆盖输入）。
func TestToMP4RejectsMP4Input(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "clip.mp4")
	if err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:d=1:r=6",
		"-pix_fmt", "yuv420p", in).Run(); err != nil {
		t.Fatalf("failed to generate test video: %v", err)
	}

	_, err := ToMP4(ToMP4Options{Input: in})
	if err == nil {
		t.Fatal("expected error when converting mp4 to itself")
	}
	if !strings.Contains(err.Error(), "already MP4") {
		t.Errorf("error should mention already MP4, got: %v", err)
	}
}

// TestToMP4IntegrationGIF 用真实 ffmpeg 把奇数高度的 GIF 转 MP4：
// 输出存在、命名正确、原文件保留，且输出宽高为偶数（H.264 要求）。
func TestToMP4IntegrationGIF(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "anim.gif")
	// 64x63：奇数高度，覆盖"去奇"路径。
	if err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=c=red:s=64x63:d=1:r=6", in).Run(); err != nil {
		t.Fatalf("failed to generate test gif: %v", err)
	}

	out, err := ToMP4(ToMP4Options{Input: in, Verbose: true})
	if err != nil {
		t.Fatalf("ToMP4 failed: %v", err)
	}
	if _, statErr := os.Stat(out); statErr != nil {
		t.Fatalf("output mp4 not created: %v", statErr)
	}
	if filepath.Base(out) != "anim.mp4" {
		t.Errorf("unexpected output name %q", out)
	}
	if filepath.Dir(out) != filepath.Dir(in) {
		t.Errorf("output dir %q != input dir %q", filepath.Dir(out), filepath.Dir(in))
	}
	if _, statErr := os.Stat(in); statErr != nil {
		t.Errorf("original input should be preserved, got: %v", statErr)
	}
	if _, lookErr := exec.LookPath("ffprobe"); lookErr != nil {
		t.Skip("ffprobe not on PATH; even-dimension assertion skipped")
	}
	if w, h := probeVideoSize(out); w <= 0 || h <= 0 || w%2 != 0 || h%2 != 0 {
		t.Errorf("expected positive even dimensions, got %dx%d", w, h)
	}
}

// TestToMP4KeepsAudio 验证视频容器（mov，带音轨）转 mp4 后音轨被保留（未传 -an）。
func TestToMP4KeepsAudio(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not on PATH; skipping integration test")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not on PATH; audio assertion skipped")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "clip.mov")
	if err := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "color=c=blue:s=64x64:d=1:r=6",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-c:a", "aac", "-shortest", in).Run(); err != nil {
		t.Fatalf("failed to generate test mov: %v", err)
	}

	out, err := ToMP4(ToMP4Options{Input: in})
	if err != nil {
		t.Fatalf("ToMP4 failed: %v", err)
	}
	probe, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "a",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0", out).Output()
	if err != nil {
		t.Fatalf("ffprobe failed: %v", err)
	}
	if !strings.Contains(string(probe), "audio") {
		t.Errorf("expected audio stream preserved, ffprobe output: %q", string(probe))
	}
}
