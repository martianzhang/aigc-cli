package wmremove

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateMask(t *testing.T) {
	mask := GenerateMask(100, 100, 80, 80, 20, 20)
	if mask.GrayAt(90, 90).Y != 255 {
		t.Error("masked area should be white")
	}
	if mask.GrayAt(0, 0).Y != 0 {
		t.Error("non-masked area should be black")
	}
}

func TestPreprocessImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	data := preprocessImage(img, 10, 10)
	if len(data) != 300 {
		t.Fatalf("len: %d", len(data))
	}
	if data[0] != 0 {
		t.Errorf("black pixel: %d", data[0])
	}
}

func TestPreprocessMask(t *testing.T) {
	mask := GenerateMask(10, 10, 5, 5, 5, 5)
	data := preprocessMask(mask, 10, 10)
	if data[5*10+5] != 0 {
		t.Errorf("masked: %d", data[5*10+5])
	}
	if data[0] != 255 {
		t.Errorf("keep: %d", data[0])
	}
}

func TestDetectFile(t *testing.T) {
	modelsDir := os.ExpandEnv("$HOME/.config/aigc-cli/models")
	libPath := filepath.Join(modelsDir, "libonnxruntime.dylib")
	if _, err := os.Stat(libPath); err != nil {
		t.Skip("ONNX Runtime not found")
	}
	modelPath := filepath.Join(modelsDir, "migan.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		t.Skip("model not found")
	}
	det, err := NewDetector(libPath, modelPath)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	defer det.Close()

	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	mask := GenerateMask(256, 256, 200, 200, 56, 56)
	out, err := det.RemoveWatermark(img, mask)
	if err != nil {
		t.Fatalf("RemoveWatermark: %v", err)
	}
	t.Logf("OK: %dx%d", out.Bounds().Dx(), out.Bounds().Dy())

	outPath := filepath.Join(os.TempDir(), "migan_test.png")
	f, _ := os.Create(outPath)
	png.Encode(f, out)
	f.Close()
	t.Logf("Saved: %s", outPath)
}
