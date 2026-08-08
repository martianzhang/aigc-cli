package depth

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestConvertAnnotateCallback 验证 Annotate 回调在写盘前被逐帧调用，且返回的
// 替换像素会覆盖深度帧（生成彩色标注视频）。需要 ffmpeg + 深度模型，缺失时跳过。
func TestConvertAnnotateCallback(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	modelsDir := filepath.Join(home, ".config", "aigc-cli", "models")
	if _, err := os.Stat(filepath.Join(modelsDir, "depth", "depth-anything-v2-small.onnx")); err != nil {
		t.Skip("depth model not installed; run `aigc-cli depth init`")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.mp4")
	// 生成 1 秒 10fps 的纯色测试视频。
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=blue:s=128x72:d=1:r=10", "-c:v", "libx264", "-pix_fmt", "yuv420p", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg gen: %v\n%s", err, out)
	}

	calls := 0
	outPath := filepath.Join(tmp, "out.mp4")
	_, err = Convert(ConvertOptions{
		Input:    src,
		Output:   outPath,
		ModelID:  DefaultModelID,
		Parallel: 1,
		Smooth:   false,
		Annotate: func(framePath string, gray *image.Gray) ([]uint8, bool) {
			calls++
			// 写盘前 framePath 应是原图（可解码）。
			f, err := os.Open(framePath)
			if err != nil {
				return nil, false
			}
			_, _, decErr := image.Decode(f)
			f.Close()
			if decErr != nil {
				t.Errorf("frame %s not decodable in callback: %v", framePath, decErr)
				return nil, false
			}
			// 返回红色像素替换深度帧。
			pix := make([]uint8, gray.Bounds().Dx()*gray.Bounds().Dy()*4)
			for i := 0; i < len(pix); i += 4 {
				pix[i] = 255
				pix[i+1], pix[i+2], pix[i+3] = 0, 0, 255
			}
			return pix, true
		},
	})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if calls == 0 {
		t.Fatal("Annotate callback never called")
	}

	// 输出视频首帧应为红色（标注帧），验证替换生效。
	frame := filepath.Join(tmp, "frame.png")
	cmd = exec.Command("ffmpeg", "-y", "-i", outPath, "-frames:v", "1", frame)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg extract: %v\n%s", err, out)
	}
	f, err := os.Open(frame)
	if err != nil {
		t.Fatalf("open frame: %v", err)
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	r, g, b, a := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
	if r>>8 < 250 || g>>8 > 10 || b>>8 > 10 || a>>8 < 250 {
		t.Errorf("center pixel = (%d,%d,%d,%d), want ~red (255,0,0,255)", r>>8, g>>8, b>>8, a>>8)
	}
}
