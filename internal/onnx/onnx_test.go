package onnx

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// localLibName mirrors onnxrt.LibPath's platform switch: the single main
// library filename expected on the current platform.
func localLibName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libonnxruntime.dylib"
	case "linux":
		return "libonnxruntime.so"
	default: // windows
		return "onnxruntime.dll"
	}
}

// ── Model constants ────────────────────────────────────────────────────────

func TestModelConstants(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"ModelInputName", ModelInputName, "pixel_values"},
		{"ModelOutputName", ModelOutputName, "logits"},
		{"ModelInputSize", ModelInputSize, 224},
		{"ModelChannels", ModelChannels, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestModelInputTensorVolume(t *testing.T) {
	// The input tensor is 1 x Channels x Size x Size; the flat element count
	// must match what NewDetector allocates in init().
	const want = 1 * 3 * 224 * 224
	got := 1 * ModelChannels * ModelInputSize * ModelInputSize
	if got != want {
		t.Errorf("input tensor volume = %d, want %d", got, want)
	}
}

// ── DefaultModelPath ───────────────────────────────────────────────────────

func TestDefaultModelPath_Join(t *testing.T) {
	tests := []struct {
		name      string
		modelsDir string
		want      string
	}{
		{"plain dir", filepath.Join("models", "aigc"), filepath.Join("models", "aigc", "model.onnx")},
		{"trailing separator", filepath.Join("models", "aigc") + string(filepath.Separator), filepath.Join("models", "aigc", "model.onnx")},
		{"empty dir", "", "model.onnx"},
		{"relative dot", ".", "model.onnx"},
		{"nested", filepath.Join("a", "b", "c"), filepath.Join("a", "b", "c", "model.onnx")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultModelPath(tt.modelsDir); got != tt.want {
				t.Errorf("DefaultModelPath(%q) = %q, want %q", tt.modelsDir, got, tt.want)
			}
		})
	}
}

func TestDefaultModelPath_AlwaysModelOnnx(t *testing.T) {
	// Whatever the directory, the basename is fixed.
	for _, dir := range []string{"", ".", "/tmp", filepath.Join("x", "y")} {
		if got := filepath.Base(DefaultModelPath(dir)); got != "model.onnx" {
			t.Errorf("DefaultModelPath(%q) basename = %q, want %q", dir, got, "model.onnx")
		}
	}
}

// ── DefaultLibPath (path construction + stat) ──────────────────────────────

func TestDefaultLibPath_Construction(t *testing.T) {
	dir := t.TempDir()
	libName := localLibName()
	libPath := filepath.Join(dir, libName)

	// A file that exists but is NOT the platform library name must be ignored.
	wrongName := "some-other-runtime.bin"
	if err := os.WriteFile(filepath.Join(dir, wrongName), []byte("no"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		dir     func() string
		wantErr bool
	}{
		{
			name:    "missing platform lib in temp dir",
			dir:     func() string { return dir },
			wantErr: true,
		},
		{
			name:    "nonexistent dir",
			dir:     func() string { return filepath.Join(dir, "does-not-exist") },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultLibPath(tt.dir())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("DefaultLibPath() = %q, want error", got)
				}
				if got != "" {
					t.Errorf("DefaultLibPath() path = %q, want empty on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DefaultLibPath() unexpected error: %v", err)
			}
		})
	}

	// Now drop the platform library in and re-check exact construction.
	if err := os.WriteFile(libPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := DefaultLibPath(dir)
	if err != nil {
		t.Fatalf("DefaultLibPath(%q) unexpected error: %v", dir, err)
	}
	if got != libPath {
		t.Errorf("DefaultLibPath(%q) = %q, want %q", dir, got, libPath)
	}
}

func TestDefaultLibPath_TrailingSeparator(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, localLibName())
	if err := os.WriteFile(libPath, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	// filepath.Join normalizes a trailing separator.
	got, err := DefaultLibPath(dir + string(filepath.Separator))
	if err != nil {
		t.Fatalf("DefaultLibPath() unexpected error: %v", err)
	}
	if got != libPath {
		t.Errorf("DefaultLibPath(trailing sep) = %q, want %q", got, libPath)
	}
}

// ── Detector.ModelPath (pure accessor, no runtime init) ────────────────────

func TestDetector_ModelPath(t *testing.T) {
	tests := []struct {
		name      string
		modelPath string
	}{
		{"plain", "model.onnx"},
		{"absolute", filepath.Join(string(filepath.Separator), "opt", "models", "model.onnx")},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Constructing the struct does not touch ONNX Runtime.
			d := &Detector{modelPath: tt.modelPath}
			if got := d.ModelPath(); got != tt.modelPath {
				t.Errorf("ModelPath() = %q, want %q", got, tt.modelPath)
			}
		})
	}
}

// ── Detector struct fields / zero-value defaults ───────────────────────────

func TestDetector_ZeroValueDefaults(t *testing.T) {
	tests := []struct {
		name  string
		field func(d *Detector) any
		want  any
	}{
		{"modelPath empty", func(d *Detector) any { return d.modelPath }, ""},
		{"libPath empty", func(d *Detector) any { return d.libPath }, ""},
		{"session nil", func(d *Detector) any { return d.session == nil }, true},
		{"input nil", func(d *Detector) any { return d.input == nil }, true},
		{"output nil", func(d *Detector) any { return d.output == nil }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Constructing the struct does not touch ONNX Runtime.
			d := &Detector{}
			if got := tt.field(d); got != tt.want {
				t.Errorf("zero Detector %s = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// ── Result struct ──────────────────────────────────────────────────────────

func TestResult_Fields(t *testing.T) {
	tests := []struct {
		name string
		rate float64
	}{
		{"zero", 0},
		{"half", 0.5},
		{"one", 1.0},
		{"out of range high", 1.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Result{AIGenRate: tt.rate}
			if r.AIGenRate != tt.rate {
				t.Errorf("AIGenRate = %v, want %v", r.AIGenRate, tt.rate)
			}
			var zero Result
			if zero.AIGenRate != 0 {
				t.Errorf("zero Result AIGenRate = %v, want 0", zero.AIGenRate)
			}
		})
	}
}

// ── Model constants consistency ────────────────────────────────────────────

func TestModelConstants_Consistency(t *testing.T) {
	if ModelInputSize <= 0 {
		t.Errorf("ModelInputSize = %d, want positive", ModelInputSize)
	}
	if ModelChannels != 3 {
		t.Errorf("ModelChannels = %d, want 3 (RGB)", ModelChannels)
	}
	if ModelInputName == "" || ModelOutputName == "" {
		t.Errorf("tensor names must be non-empty: input=%q output=%q", ModelInputName, ModelOutputName)
	}
	if ModelInputName == ModelOutputName {
		t.Error("input and output tensor names must differ")
	}
}

// ── NewDetector (stat validation only, no runtime load) ────────────────────

func TestNewDetector_StatErrors(t *testing.T) {
	dir := t.TempDir()
	existingFile := filepath.Join(dir, "fake-lib.bin")
	if err := os.WriteFile(existingFile, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "does-not-exist")

	tests := []struct {
		name        string
		libPath     string
		modelPath   string
		wantErrPart string
	}{
		{"missing lib and model", missing + "-lib", missing + "-model", "library not found"},
		{"lib present model missing", existingFile, missing + "-model", "model not found"},
		{"relative missing paths", "no-such-lib", "no-such-model", "library not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := NewDetector(tt.libPath, tt.modelPath)
			if err == nil {
				t.Fatalf("NewDetector(%q, %q) = %v, want error", tt.libPath, tt.modelPath, d)
			}
			if !strings.Contains(err.Error(), tt.wantErrPart) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErrPart)
			}
			if d != nil {
				t.Errorf("detector = %v, want nil on stat error", d)
			}
		})
	}
}

// ── DetectFile open/decode errors (no runtime, no inference) ───────────────

func TestDetectFile_OpenError(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"missing file", filepath.Join("no", "such", "file.png")},
		{"empty path", ""},
		{"directory", "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Detector{}
			result, err := d.DetectFile(tt.path)
			if err == nil {
				t.Fatalf("DetectFile(%q) = %v, want error", tt.path, result)
			}
			if result != nil {
				t.Errorf("result = %v, want nil on open/decode error", result)
			}
		})
	}
}

// ── Softmax table-driven properties ────────────────────────────────────────

func TestSoftmax_TableDrivenProperties(t *testing.T) {
	tests := []struct {
		name   string
		logits []float32
	}{
		{"two class", []float32{0, 1}},
		{"three class", []float32{1, 2, 3}},
		{"all zero", []float32{0, 0, 0}},
		{"large negatives", []float32{-100, -200}},
		{"mixed signs", []float32{-5, 0, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probs := Softmax(tt.logits)
			if len(probs) != len(tt.logits) {
				t.Fatalf("len(probs) = %d, want %d", len(probs), len(tt.logits))
			}
			var sum float64
			for i, p := range probs {
				if p < 0 || p > 1 {
					t.Errorf("probs[%d] = %f, want [0,1]", i, p)
				}
				sum += float64(p)
			}
			if math.Abs(sum-1) > 1e-5 {
				t.Errorf("sum(probs) = %f, want 1", sum)
			}
		})
	}
}

// ── Integration (skipped unless ONNX Runtime + model are installed) ────────

func TestDetectFile(t *testing.T) {
	modelsDir := os.ExpandEnv("$HOME/.config/aigc-cli/models")

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
