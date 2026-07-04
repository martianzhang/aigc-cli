// Package onnx provides a pure-Go wrapper around the ONNX Runtime shared library
// for AIGC image detection. Zero CGO dependency.
package onnx

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	ort "github.com/amikos-tech/pure-onnx/ort"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

const (
	// ModelInputName is the expected ONNX model input tensor name.
	ModelInputName = "pixel_values"
	// ModelOutputName is the expected ONNX model output tensor name.
	ModelOutputName = "logits"
	// ModelInputSize is the expected image size (width/height) in pixels.
	ModelInputSize = 224
	// ModelChannels is the expected number of color channels.
	ModelChannels = 3
)

// Result holds the AIGC detection inference result.
type Result struct {
	FakeScore float64 `json:"fake_score"`
	RealScore float64 `json:"real_score"`
	IsFake    bool    `json:"is_fake"`
	IsReal    bool    `json:"is_real"`
}

// Detector manages the ONNX Runtime lifecycle and inference session.
type Detector struct {
	modelPath string
	libPath   string
	session   *ort.AdvancedSession
	input     *ort.Tensor[float32]
	output    *ort.Tensor[float32]
}

// NewDetector creates a new ONNX Detector, loading the runtime and model.
// Call Close() when done to release resources.
func NewDetector(libPath, modelPath string) (*Detector, error) {
	if _, err := os.Stat(libPath); err != nil {
		return nil, fmt.Errorf("onnx runtime library not found: %w", err)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}

	d := &Detector{
		libPath:   libPath,
		modelPath: modelPath,
	}

	if err := d.init(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Detector) init() error {
	if err := ort.SetSharedLibraryPath(d.libPath); err != nil {
		return fmt.Errorf("set library path: %w", err)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("initialize environment: %w", err)
	}

	// Create fixed-size input/output tensors
	shape := ort.NewShape(1, ModelChannels, ModelInputSize, ModelInputSize)
	totalSize := 1 * ModelChannels * ModelInputSize * ModelInputSize
	inputData := make([]float32, totalSize)
	var err error
	d.input, err = ort.NewTensor(shape, inputData)
	if err != nil {
		ort.DestroyEnvironment()
		return fmt.Errorf("create input tensor: %w", err)
	}

	outputShape := ort.NewShape(1, 2)
	outputData := make([]float32, 2)
	d.output, err = ort.NewTensor(outputShape, outputData)
	if err != nil {
		d.input.Destroy()
		ort.DestroyEnvironment()
		return fmt.Errorf("create output tensor: %w", err)
	}

	d.session, err = ort.NewAdvancedSession(
		d.modelPath,
		[]string{ModelInputName},
		[]string{ModelOutputName},
		[]ort.Value{d.input},
		[]ort.Value{d.output},
		nil,
	)
	if err != nil {
		d.output.Destroy()
		d.input.Destroy()
		ort.DestroyEnvironment()
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}

// Detect runs AIGC detection on the given image.
// The image must be decoded (can be any format supported by image.Decode).
func (d *Detector) Detect(img image.Image) (*Result, error) {
	// Preprocess: resize to 224x224 and normalize
	pixels := Preprocess(img, ModelInputSize)

	// Copy pixels into input tensor
	data := d.input.GetData()
	if len(data) != len(pixels) {
		return nil, fmt.Errorf("tensor size mismatch: got %d want %d", len(data), len(pixels))
	}
	copy(data, pixels)

	if err := d.session.Run(); err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	outData := d.output.GetData()
	if len(outData) < 2 {
		return nil, fmt.Errorf("unexpected output size: %d", len(outData))
	}

	// Apply softmax to get probabilities
	realScore := Softmax(outData)[0]
	fakeScore := Softmax(outData)[1]

	return &Result{
		FakeScore: float64(fakeScore),
		RealScore: float64(realScore),
		IsFake:    fakeScore > realScore,
		IsReal:    realScore >= fakeScore,
	}, nil
}

// DetectFile loads an image file and runs AIGC detection.
func (d *Detector) DetectFile(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	return d.Detect(img)
}

// Close releases all ONNX Runtime resources.
func (d *Detector) Close() {
	if d.session != nil {
		d.session.Destroy()
	}
	if d.output != nil {
		d.output.Destroy()
	}
	if d.input != nil {
		d.input.Destroy()
	}
	ort.DestroyEnvironment()
}

// DefaultLibPath returns the expected path for the ONNX Runtime shared library
// based on the OS and a given models directory.
func DefaultLibPath(modelsDir string) (string, error) {
	candidates := []string{
		filepath.Join(modelsDir, "onnxruntime.dll"),
		filepath.Join(modelsDir, "libonnxruntime.so"),
		filepath.Join(modelsDir, "libonnxruntime.dylib"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", errors.New("ONNX Runtime library not found")
}

// DefaultModelPath returns the expected path for the ONNX model file.
func DefaultModelPath(modelsDir string) string {
	return filepath.Join(modelsDir, "model.onnx")
}
