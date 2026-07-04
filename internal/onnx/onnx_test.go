package onnx

import (
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFile(t *testing.T) {
	modelsDir := os.ExpandEnv("$HOME/.config/apimart/models2")

	libCandidates := []string{
		filepath.Join(modelsDir, "onnxruntime-win-x64-1.27.0", "lib", "onnxruntime.dll"),
		filepath.Join(modelsDir, "onnxruntime.dll"),
		filepath.Join(modelsDir, "libonnxruntime.so"),
		filepath.Join(modelsDir, "libonnxruntime.dylib"),
	}
	var libPath string
	for _, c := range libCandidates {
		if _, err := os.Stat(c); err == nil {
			libPath = c
			break
		}
	}
	if libPath == "" {
		t.Skip("ONNX Runtime library not found in", modelsDir)
	}

	modelPath := filepath.Join(modelsDir, "model.onnx")
	if _, err := os.Stat(modelPath); err != nil {
		t.Skip("model.onnx not found")
	}

	// Test 1: dummy image
	d, err := NewDetector(libPath, modelPath)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	defer d.Close()

	img := image.NewRGBA(image.Rect(0, 0, 224, 224))
	result, err := d.Detect(img)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	t.Logf("Dummy -> Fake=%.4f Real=%.4f", result.FakeScore, result.RealScore)
	if result.FakeScore+result.RealScore < 0.99 {
		t.Errorf("Scores should sum ~1.0, got %f", result.FakeScore+result.RealScore)
	}

	// Test 2: real image
	downloadPath := os.ExpandEnv("$HOME/Downloads/技术架构方法环境方法论.png")
	if _, err := os.Stat(downloadPath); err == nil {
		result, err := d.DetectFile(downloadPath)
		if err != nil {
			t.Fatalf("DetectFile: %v", err)
		}
		label := "REAL"
		if result.IsFake {
			label = "FAKE"
		}
		t.Logf("技术架构方法环境方法论.png -> Fake=%.4f Real=%.4f [%s]",
			result.FakeScore, result.RealScore, label)
	}
}
