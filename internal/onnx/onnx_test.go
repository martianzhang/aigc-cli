package onnx

import (
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFile(t *testing.T) {
	modelsDir := os.ExpandEnv("$HOME/.config/apimart/models")

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

	// Try large model first, then small
	var modelPath string
	for _, name := range []string{"model-large.onnx", "model-small.onnx"} {
		p := filepath.Join(modelsDir, name)
		if _, err := os.Stat(p); err == nil {
			modelPath = p
			break
		}
	}
	if modelPath == "" {
		t.Skip("model.onnx not found")
	}

	d, err := NewDetector(libPath, modelPath)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	defer d.Close()

	// Test 1: dummy image - should return a score
	img := image.NewRGBA(image.Rect(0, 0, 224, 224))
	result, err := d.Detect(img)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	t.Logf("Dummy -> AIGenRate=%.4f", result.AIGenRate)
	if result.AIGenRate < 0 || result.AIGenRate > 1 {
		t.Errorf("AIGenRate out of range [0,1]: %f", result.AIGenRate)
	}
}
