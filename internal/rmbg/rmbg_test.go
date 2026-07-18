package rmbg

import (
	"image"
	"math"
	"os"
	"testing"
)

func TestPreprocess(t *testing.T) {
	// Create a small test image
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Preprocess to 64x64 (smaller for test speed)
	pixels := Preprocess(img, 64)

	expectedLen := 3 * 64 * 64
	if len(pixels) != expectedLen {
		t.Errorf("Preprocess output length: got %d, want %d", len(pixels), expectedLen)
	}

	// Check CHW layout: first 64*64 values are R channel
	// For a black image (0,0,0), normalized = (0 - mean) / std
	expectedR := -imagenetMean[0] / imagenetStd[0]
	for i := 0; i < 64*64; i++ {
		if math.Abs(float64(pixels[i]-expectedR)) > 0.001 {
			t.Errorf("R channel pixel %d: got %f, want %f", i, pixels[i], expectedR)
			break
		}
	}
}

func TestPreprocessWhite(t *testing.T) {
	// White image test
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for i := range img.Pix {
		img.Pix[i] = 255
	}

	pixels := Preprocess(img, 10)

	// For white (255,255,255), normalized = (1.0 - mean) / std
	expected := (1.0 - imagenetMean[0]) / imagenetStd[0]
	if math.Abs(float64(pixels[0]-expected)) > 0.001 {
		t.Errorf("White pixel: got %f, want %f", pixels[0], expected)
	}
}

func TestSigmoid(t *testing.T) {
	tests := []struct {
		input    float32
		expected float32
	}{
		{0, 0.5},
		{1, 0.7310586},
		{-1, 0.2689414},
		{10, 0.9999546},
		{-10, 0.0000454},
		{100, 1.0},  // clamped
		{-100, 0.0}, // clamped
	}

	for _, tt := range tests {
		result := Sigmoid([]float32{tt.input})
		diff := float64(result[0] - tt.expected)
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.001 {
			t.Errorf("Sigmoid(%f): got %f, want %f", tt.input, result[0], tt.expected)
		}
	}
}

func TestResizeMask(t *testing.T) {
	// Create a 4x4 mask with known values
	srcW, srcH := 4, 4
	mask := make([]float32, srcW*srcH)
	for y := 0; y < srcH; y++ {
		for x := 0; x < srcW; x++ {
			mask[y*srcW+x] = float32(x+y) / 6.0 // values 0 to 1
		}
	}

	// Resize to 8x8
	dstW, dstH := 8, 8
	result := ResizeMask(mask, srcW, srcH, dstW, dstH)

	if len(result) != dstW*dstH {
		t.Errorf("ResizeMask output length: got %d, want %d", len(result), dstW*dstH)
	}

	// Corner pixel should match (nearest-neighbor)
	if result[0] != 0 {
		t.Errorf("Top-left pixel: got %d, want 0", result[0])
	}
}

func TestResizeMaskSameSize(t *testing.T) {
	srcW, srcH := 16, 16
	mask := make([]float32, srcW*srcH)
	for i := range mask {
		mask[i] = 0.5
	}

	result := ResizeMask(mask, srcW, srcH, srcW, srcH)
	if len(result) != len(mask) {
		t.Errorf("Same-size ResizeMask length: got %d, want %d", len(result), len(mask))
	}
	for i, v := range result {
		if v != 128 { // 0.5 * 255 = 127.5 → 127
			// Allow rounding difference (0.5*255 = 127.5 → int cast = 127)
			if v != 127 && v != 128 {
				t.Errorf("Same-size pixel %d: got %d, want 127 or 128", i, v)
			}
		}
	}
}

func TestClampU8(t *testing.T) {
	tests := []struct {
		input    float32
		expected uint8
	}{
		{-1, 0},
		{0, 0},
		{128, 128},
		{255, 255},
		{300, 255},
	}

	for _, tt := range tests {
		result := clampU8(tt.input)
		if result != tt.expected {
			t.Errorf("clampU8(%f): got %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestApplyAlpha(t *testing.T) {
	w, h := 4, 4
	rgba := make([]uint8, w*h*4)
	alpha := make([]uint8, w*h)

	// Fill RGBA with red
	for i := 0; i < w*h*4; i += 4 {
		rgba[i] = 255   // R
		rgba[i+1] = 0   // G
		rgba[i+2] = 0   // B
		rgba[i+3] = 255 // A
	}

	// Set alpha: top-left half transparent, bottom-right half opaque
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 && y < h/2 {
				alpha[y*w+x] = 64 // semi-transparent
			} else {
				alpha[y*w+x] = 255 // opaque
			}
		}
	}

	result := ApplyAlpha(rgba, w, h, alpha)

	// Check top-left pixel (semi-transparent)
	if result[0] != 255 || result[1] != 0 || result[2] != 0 || result[3] != 64 {
		t.Errorf("Top-left pixel: got [%d,%d,%d,%d], want [255,0,0,64]",
			result[0], result[1], result[2], result[3])
	}

	// Check bottom-right pixel (opaque)
	idx := 3*4 + 3*4 // bottom-right
	if result[idx] != 255 || result[idx+1] != 0 || result[idx+2] != 0 || result[idx+3] != 255 {
		t.Errorf("Bottom-right pixel: got [%d,%d,%d,%d], want [255,0,0,255]",
			result[idx], result[idx+1], result[idx+2], result[idx+3])
	}
}

// TestDetectFile is an integration test that requires the actual model and ONNX Runtime.
// It follows the same pattern as internal/onnx/onnx_test.go.
func TestDetectFile(t *testing.T) {
	modelsDir := os.ExpandEnv("$HOME/.config/aigc-cli/models")
	if _, err := os.Stat(modelsDir); err != nil {
		t.Skip("models directory not found:", modelsDir)
	}

	libPath, err := DefaultLibPath(modelsDir)
	if err != nil {
		t.Skip("ONNX Runtime library not found in", modelsDir)
	}

	modelPath := DefaultModelPath(modelsDir)
	if _, err := os.Stat(modelPath); err != nil {
		t.Skip("RMBG model not found:", modelPath)
	}

	d, err := NewDetector(libPath, modelPath)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	defer d.Close()

	// Test with a dummy image
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	nrga, gray, err := d.RemoveBackground(img)
	if err != nil {
		t.Fatalf("RemoveBackground: %v", err)
	}

	if nrga.Bounds().Dx() != 256 || nrga.Bounds().Dy() != 256 {
		t.Errorf("output size: got %dx%d, want 256x256", nrga.Bounds().Dx(), nrga.Bounds().Dy())
	}

	if gray.Bounds().Dx() != 256 || gray.Bounds().Dy() != 256 {
		t.Errorf("mask size: got %dx%d, want 256x256", gray.Bounds().Dx(), gray.Bounds().Dy())
	}

	t.Logf("RemoveBackground OK: %dx%d", nrga.Bounds().Dx(), nrga.Bounds().Dy())
}
