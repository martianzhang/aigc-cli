package rmbg

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"

	ort "github.com/amikos-tech/pure-onnx/ort"
)

// Default input/output tensor names for RMBG 2.0 ONNX model.
// The model has 4 multi-scale decoder outputs; we request all 4 and use the last.
const (
	ModelInputName  = "pixel_values"
	ModelOutputName = "alphas" // final decoder output (full-res alpha)
)

// OutputShape is the expected output tensor shape: [1, 1, 1024, 1024].
var OutputShape = ort.NewShape(1, 1, ModelInputSize, ModelInputSize)

// Result holds the background removal result metadata.
type Result struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Detector 管理 ONNX Runtime 生命周期和推理会话。
// 使用纯 Go (pure-onnx)，无 CGo 依赖。
type Detector struct {
	modelPath string
	libPath   string
	session   *ort.AdvancedSession
	input     *ort.Tensor[float32]
	output    *ort.Tensor[float32]
}

// NewDetector 创建新的 RMBG Detector，加载 ONNX Runtime 和模型文件。
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
	_ = ort.SetLogLevel(ort.LoggingLevelError)
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("initialize environment: %w", err)
	}

	// Create fixed-size input tensor: [1, 3, 1024, 1024]
	inputShape := ort.NewShape(1, ModelChannels, ModelInputSize, ModelInputSize)
	totalInput := 1 * ModelChannels * ModelInputSize * ModelInputSize
	inputData := make([]float32, totalInput)
	var err error
	d.input, err = ort.NewTensor(inputShape, inputData)
	if err != nil {
		ort.DestroyEnvironment()
		return fmt.Errorf("create input tensor: %w", err)
	}

	// Create fixed-size output tensor: [1, 1, 1024, 1024]
	totalOutput := 1 * 1 * ModelInputSize * ModelInputSize
	outputData := make([]float32, totalOutput)
	d.output, err = ort.NewTensor(OutputShape, outputData)
	if err != nil {
		d.input.Destroy()
		ort.DestroyEnvironment()
		return fmt.Errorf("create output tensor: %w", err)
	}

	opts := ort.NewCUDASessionOptions()
	d.session, err = ort.NewAdvancedSession(
		d.modelPath,
		[]string{ModelInputName},
		[]string{ModelOutputName},
		[]ort.Value{d.input},
		[]ort.Value{d.output},
		opts,
	)
	if opts != nil {
		opts.Destroy()
	}
	if err != nil {
		d.output.Destroy()
		d.input.Destroy()
		ort.DestroyEnvironment()
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}

// RemoveBackground 对图片进行语义分割去背。
// 返回 NRGBA 图片（透明背景）和灰度 alpha 遮罩。
// alpha 遮罩可用于后续 autocrop/shadow 等后处理。
func (d *Detector) RemoveBackground(img image.Image) (*image.NRGBA, *image.Gray, error) {
	b := img.Bounds()
	origW := b.Dx()
	origH := b.Dy()

	// ── 1. Preprocess ──
	pixels := Preprocess(img, ModelInputSize)

	// Copy to input tensor
	data := d.input.GetData()
	if len(data) != len(pixels) {
		return nil, nil, fmt.Errorf("tensor size mismatch: got %d want %d", len(data), len(pixels))
	}
	copy(data, pixels)

	// ── 2. Run inference ──
	if err := d.session.Run(); err != nil {
		return nil, nil, fmt.Errorf("inference failed: %w", err)
	}

	// ── 3. Read output ──
	outData := d.output.GetData()
	expectedOut := 1 * 1 * ModelInputSize * ModelInputSize
	if len(outData) < expectedOut {
		return nil, nil, fmt.Errorf("unexpected output size: got %d want >=%d", len(outData), expectedOut)
	}

	// ── 4. Postprocess ──
	// The ONNX graph has sigmoid fused in, so output is already in [0, 1].
	// Resize mask from 1024x1024 to original image dimensions.
	alpha := ResizeMask(outData, ModelInputSize, ModelInputSize, origW, origH)

	// Build RGBA pixel buffer from original image
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	pix := rgba.Pix

	// Apply alpha to get NRGBA with transparency
	outPixels := ApplyAlpha(pix, origW, origH, alpha)

	outImg := image.NewNRGBA(image.Rect(0, 0, origW, origH))
	copy(outImg.Pix, outPixels)
	outImg.Stride = origW * 4

	// Build grayscale alpha mask for downstream usage
	gray := image.NewGray(image.Rect(0, 0, origW, origH))
	copy(gray.Pix, alpha)

	return outImg, gray, nil
}

// ModelPath 返回当前使用的 ONNX 模型路径。
func (d *Detector) ModelPath() string { return d.modelPath }

// Close 释放所有 ONNX Runtime 资源。
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

// SavePNG saves image.Image to a PNG file.
func SavePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// DefaultLibPath 返回 ONNX Runtime 共享库的路径。
// 优先查找 GPU 版本（libonnxruntime_gpu.*），找不到则回退到 CPU 版本。
func DefaultLibPath(modelsDir string) (string, error) {
	// GPU variants first (if available)
	gpuCandidates := []string{
		modelsDir + "/libonnxruntime_gpu.dylib",
		modelsDir + "/libonnxruntime_gpu.so",
		modelsDir + "/onnxruntime_gpu.dll",
	}
	for _, c := range gpuCandidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	// CPU fallback
	candidates := []string{
		modelsDir + "/libonnxruntime.dylib",
		modelsDir + "/libonnxruntime.so",
		modelsDir + "/onnxruntime.dll",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("ONNX Runtime library not found in %s", modelsDir)
}

// DefaultModelPath 返回默认的 RMBG 2.0 模型路径。
func DefaultModelPath(modelsDir string) string {
	return modelsDir + "/model-rmbg-2.0.onnx"
}
