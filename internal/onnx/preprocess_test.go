package onnx

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPreprocess_OutputSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	pixels := Preprocess(img, 224)
	expectedLen := 3 * 224 * 224 // CHW format
	if len(pixels) != expectedLen {
		t.Errorf("Preprocess: expected len %d, got %d", expectedLen, len(pixels))
	}
}

func TestPreprocess_NormalizedRange(t *testing.T) {
	// All-white image → all pixels should be 1.0
	img := image.NewRGBA(image.Rect(0, 0, 224, 224))
	for y := 0; y < 224; y++ {
		for x := 0; x < 224; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	pixels := Preprocess(img, 224)
	for i, v := range pixels {
		if v < 0 || v > 1.001 {
			t.Errorf("pixel[%d] = %f, want [0,1]", i, v)
		}
	}
}

func TestPreprocess_AllBlack(t *testing.T) {
	// All-black image → all pixels should be 0.0
	img := image.NewRGBA(image.Rect(0, 0, 224, 224))
	pixels := Preprocess(img, 224)
	for i, v := range pixels {
		if v > 0.001 {
			t.Errorf("black pixel[%d] = %f, want 0", i, v)
		}
	}
}

func TestPreprocess_ResizeLarger(t *testing.T) {
	// 50x50 image resized to 224x224
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	pixels := Preprocess(img, 224)
	expectedLen := 3 * 224 * 224
	if len(pixels) != expectedLen {
		t.Errorf("resize larger: expected len %d, got %d", expectedLen, len(pixels))
	}
}

func TestPreprocess_ResizeSmaller(t *testing.T) {
	// 500x500 image resized to 224x224
	img := image.NewRGBA(image.Rect(0, 0, 500, 500))
	pixels := Preprocess(img, 224)
	expectedLen := 3 * 224 * 224
	if len(pixels) != expectedLen {
		t.Errorf("resize smaller: expected len %d, got %d", expectedLen, len(pixels))
	}
}

func TestPreprocess_ExactSize(t *testing.T) {
	// Already 224x224 → no resize needed
	img := image.NewRGBA(image.Rect(0, 0, 224, 224))
	for y := 0; y < 224; y++ {
		for x := 0; x < 224; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}
	pixels := Preprocess(img, 224)
	expectedLen := 3 * 224 * 224
	if len(pixels) != expectedLen {
		t.Errorf("exact size: expected len %d, got %d", expectedLen, len(pixels))
	}
}

func TestPreprocess_GrayscaleImage(t *testing.T) {
	// Gray image → all channels should have same value
	img := image.NewGray(image.Rect(0, 0, 224, 224))
	for y := 0; y < 224; y++ {
		for x := 0; x < 224; x++ {
			img.SetGray(x, y, color.Gray{Y: 128})
		}
	}
	pixels := Preprocess(img, 224)
	stride := 224 * 224
	// All channels should be ~0.5
	for i := 0; i < stride; i++ {
		if pixels[i] < 0.49 || pixels[i] > 0.51 {
			t.Errorf("R channel pixel[%d] = %f, want ~0.5", i, pixels[i])
		}
		if pixels[i+stride] < 0.49 || pixels[i+stride] > 0.51 {
			t.Errorf("G channel pixel[%d] = %f, want ~0.5", i+stride, pixels[i+stride])
		}
	}
}

func TestPreprocess_NilImage(t *testing.T) {
	// Preprocess does not guard against nil images — img.Bounds() panics.
	// This test documents that behavior. If nil-safety is added later,
	// update this test.
	defer func() {
		if r := recover(); r == nil {
			t.Error("Preprocess(nil) should panic on nil image")
		}
	}()
	_ = Preprocess(nil, 224)
}

func TestPreprocess_DifferentTargetSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for _, size := range []int{64, 128, 256} {
		pixels := Preprocess(img, size)
		expectedLen := 3 * size * size
		if len(pixels) != expectedLen {
			t.Errorf("targetSize=%d: expected len %d, got %d", size, expectedLen, len(pixels))
		}
	}
}

func TestPreprocess_RedChannelOnly(t *testing.T) {
	// Pure red image → R=1.0, G=0.0, B=0.0
	img := image.NewRGBA(image.Rect(0, 0, 224, 224))
	for y := 0; y < 224; y++ {
		for x := 0; x < 224; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	pixels := Preprocess(img, 224)
	stride := 224 * 224

	// Check a few random positions for each channel
	offsets := []int{0, 100, stride, stride + 100, 2 * stride, 2*stride + 100}
	expected := []float32{1.0, 1.0, 0.0, 0.0, 0.0, 0.0}
	for i, off := range offsets {
		if pixels[off] < expected[i]-0.01 || pixels[off] > expected[i]+0.01 {
			t.Errorf("offset %d: pixel = %f, want ~%f", off, pixels[off], expected[i])
		}
	}
}

// ── Softmax tests ──────────────────────────────────────────────────────────

func TestSoftmax_SumToOne(t *testing.T) {
	logits := []float32{2.0, 1.0, 0.1}
	probs := Softmax(logits)

	var sum float64
	for _, p := range probs {
		sum += float64(p)
	}
	if math.Abs(sum-1.0) > 1e-5 {
		t.Errorf("Softmax sum = %f, want 1.0", sum)
	}
}

func TestSoftmax_TwoClass(t *testing.T) {
	logits := []float32{0.0, 2.0}
	probs := Softmax(logits)

	// Second class should have higher probability
	if probs[1] <= probs[0] {
		t.Errorf("Softmax([0,2]): probs[1]=%f should be > probs[0]=%f", probs[1], probs[0])
	}
}

func TestSoftmax_EqualInputs(t *testing.T) {
	logits := []float32{1.0, 1.0, 1.0}
	probs := Softmax(logits)

	// All outputs should be equal (≈0.333)
	for i, p := range probs {
		if math.Abs(float64(p)-1.0/3.0) > 1e-5 {
			t.Errorf("Softmax equal: probs[%d] = %f, want ~0.333", i, p)
		}
	}
}

func TestSoftmax_NegativeInputs(t *testing.T) {
	logits := []float32{-1.0, -2.0, -3.0}
	probs := Softmax(logits)

	var sum float64
	for _, p := range probs {
		sum += float64(p)
	}
	if math.Abs(sum-1.0) > 1e-5 {
		t.Errorf("Softmax negative sum = %f, want 1.0", sum)
	}
	// First class should have highest probability
	if probs[0] <= probs[1] || probs[0] <= probs[2] {
		t.Errorf("Softmax([-1,-2,-3]): probs[0]=%f should be highest", probs[0])
	}
}

func TestSoftmax_LargeValues(t *testing.T) {
	logits := []float32{1000.0, 999.0, 998.0}
	probs := Softmax(logits)

	var sum float64
	for _, p := range probs {
		sum += float64(p)
	}
	if math.Abs(sum-1.0) > 1e-5 {
		t.Errorf("Softmax large values sum = %f, want 1.0", sum)
	}
	// First class should dominate
	if probs[0] < 0.5 {
		t.Errorf("Softmax([1000,999,998]): probs[0]=%f should dominate", probs[0])
	}
}

func TestSoftmax_SingleElement(t *testing.T) {
	logits := []float32{42.0}
	probs := Softmax(logits)
	if len(probs) != 1 || probs[0] != 1.0 {
		t.Errorf("Softmax single: got %v, want [1.0]", probs)
	}
}

func TestSoftmax_Empty(t *testing.T) {
	// Softmax does not guard against empty input — logits[0] panics.
	// This documents the behavior.
	defer func() {
		if r := recover(); r == nil {
			t.Error("Softmax(nil) should panic on empty input")
		}
	}()
	Softmax(nil)
}

// ── DefaultLibPath tests ───────────────────────────────────────────────────

func TestDefaultLibPath_NotFound(t *testing.T) {
	path, err := DefaultLibPath(t.TempDir())
	if err == nil {
		t.Errorf("DefaultLibPath in empty dir: expected error, got path %q", path)
	}
}

func TestDefaultLibPath_found(t *testing.T) {
	dir := t.TempDir()
	// Create the expected library filename for the current platform.
	var libName string
	switch runtime.GOOS {
	case "windows":
		libName = "onnxruntime.dll"
	case "darwin":
		libName = "libonnxruntime.dylib"
	default:
		libName = "libonnxruntime.so"
	}
	libPath := filepath.Join(dir, libName)
	if err := os.WriteFile(libPath, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}
	path, err := DefaultLibPath(dir)
	if err != nil {
		t.Errorf("DefaultLibPath for %s: unexpected error %v", libName, err)
	}
	if path != libPath {
		t.Errorf("DefaultLibPath = %q, want %q", path, libPath)
	}
}

func TestDefaultModelPath(t *testing.T) {
	dir := "/some/models/dir"
	path := DefaultModelPath(dir)
	expected := filepath.Join(dir, "model.onnx")
	if path != expected {
		t.Errorf("DefaultModelPath = %q, want %q", path, expected)
	}
}
