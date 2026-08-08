package depth

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"

	ort "github.com/amikos-tech/pure-onnx/ort"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

// ortEnvOnce 保证 SetSharedLibraryPath 进程内只执行一次。
// ONNX Runtime 环境是全局单例（refCount 管理），多个 Detector 共享同一
// 库路径；重复 SetSharedLibraryPath 会在环境已初始化后报错。
var ortEnvOnce sync.Once

// Default input/output tensor names for the Depth Anything V2 ONNX model.
const (
	ModelInputName  = "pixel_values"
	ModelOutputName = "predicted_depth"
)

// OutputShape is the expected output tensor shape: [1, 518, 518]
// (inverse depth, no channel dimension).
var OutputShape = ort.NewShape(1, ModelInputSize, ModelInputSize)

// Detector 管理 ONNX Runtime 生命周期和推理会话。
// 使用纯 Go (pure-onnx)，无 CGo 依赖。
type Detector struct {
	modelPath string
	libPath   string
	inputSize int // inference resolution (short side), 14-aligned
	session   *ort.AdvancedSession
	input     *ort.Tensor[float32]
	output    *ort.Tensor[float32]
}

// NewDetector 创建新的深度估计 Detector，加载 ONNX Runtime 和模型文件。
func NewDetector(libPath, modelPath string) (*Detector, error) {
	return NewDetectorWithSize(libPath, modelPath, ModelInputSize)
}

// NewDetectorWithSize 与 NewDetector 相同，但指定推理分辨率（短边，14 对齐）。
// 更小的输入（如 378）显著加快推理，质量损失有限，适合 CPU 上处理长视频。
func NewDetectorWithSize(libPath, modelPath string, inputSize int) (*Detector, error) {
	return newDetector(libPath, modelPath, inputSize, optimalThreads())
}

// NewDetectorWithSizeThreads 与 NewDetectorWithSize 相同，但显式指定
// ONNX Runtime intra-op 线程数。并行推理场景（视频帧级并行）用较少的
// 线程数创建多个 Detector，避免线程过订阅。
func NewDetectorWithSizeThreads(libPath, modelPath string, inputSize, threads int) (*Detector, error) {
	if threads < 1 {
		threads = 1
	}
	return newDetector(libPath, modelPath, inputSize, threads)
}

func newDetector(libPath, modelPath string, inputSize, threads int) (*Detector, error) {
	if _, err := os.Stat(libPath); err != nil {
		return nil, fmt.Errorf("onnx runtime library not found: %w", err)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}
	if inputSize <= 0 || inputSize%ModelPatchSize != 0 {
		return nil, fmt.Errorf("input size %d must be a positive multiple of %d", inputSize, ModelPatchSize)
	}

	d := &Detector{
		libPath:   libPath,
		modelPath: modelPath,
		inputSize: inputSize,
	}

	if err := d.init(threads); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Detector) init(threads int) error {
	var envErr error
	ortEnvOnce.Do(func() {
		envErr = ort.SetSharedLibraryPath(d.libPath)
		if envErr == nil {
			_ = ort.SetLogLevel(ort.LoggingLevelError)
			envErr = ort.InitializeEnvironment()
		}
	})
	if envErr != nil {
		return fmt.Errorf("initialize ort environment: %w", envErr)
	}

	n := d.inputSize

	// Create fixed-size input tensor: [1, 3, n, n]
	inputShape := ort.NewShape(1, ModelChannels, int64(n), int64(n))
	totalInput := 1 * ModelChannels * n * n
	inputData := make([]float32, totalInput)
	var err error
	d.input, err = ort.NewTensor(inputShape, inputData)
	if err != nil {
		ort.DestroyEnvironment()
		return fmt.Errorf("create input tensor: %w", err)
	}

	// Create fixed-size output tensor: [1, n, n]
	totalOutput := 1 * n * n
	outputData := make([]float32, totalOutput)
	d.output, err = ort.NewTensor(ort.NewShape(1, int64(n), int64(n)), outputData)
	if err != nil {
		d.input.Destroy()
		ort.DestroyEnvironment()
		return fmt.Errorf("create output tensor: %w", err)
	}

	opts := ort.NewCUDASessionOptions()
	if opts == nil {
		// CPU fallback. On arm64, ORT's KleidiAI GEMM backend (default) allocates
		// a fresh ~6MB working buffer on every Run() and never frees it, so a
		// 100-frame conversion leaks ~1.4GB. Disabling it keeps RSS flat
		// (measured: 351MB → 452MB over 100 runs vs +1.4GB without).
		opts, err = ort.NewSessionOptions()
		if err == nil {
			_ = opts.AddConfigEntry("mlas.disable_kleidiai", "1")
			// 按机器配置自适应 intra-op 线程数（实测最优 ≈ 性能核 × 2，
			// 全核/超线程会因线程竞争变慢）；并行推理场景由调用方指定更小值。
			_ = opts.SetIntraOpNumThreads(threads)
		}
	}
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

// Predict 对一帧图片进行单目深度估计，返回 8-bit 灰度深度图
// （尺寸与原图一致，近亮远暗）。
func (d *Detector) Predict(img image.Image) (*image.Gray, error) {
	b := img.Bounds()
	origW := b.Dx()
	origH := b.Dy()
	n := d.inputSize

	// ── 1. Preprocess (aspect-preserving, long side = n, padded) ──
	pixels, crop := PreprocessCrop(img, n)

	// Copy to input tensor
	data := d.input.GetData()
	if len(data) != len(pixels) {
		return nil, fmt.Errorf("tensor size mismatch: got %d want %d", len(data), len(pixels))
	}
	copy(data, pixels)

	// ── 2. Run inference ──
	if err := d.session.Run(); err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	// ── 3. Read output ──
	outData := d.output.GetData()
	expectedOut := 1 * n * n
	if len(outData) < expectedOut {
		return nil, fmt.Errorf("unexpected output size: got %d want >=%d", len(outData), expectedOut)
	}

	// ── 4. Postprocess: crop pad borders, then inverse depth → grayscale ──
	return DepthToGrayCrop(outData, n, crop, origW, origH), nil
}

// PredictColor 与 Predict 相同，但返回 Spectral_r 彩色深度图
// （近处暖色红/橙，远处冷色蓝/紫）。
func (d *Detector) PredictColor(img image.Image) (*image.RGBA, error) {
	b := img.Bounds()
	origW := b.Dx()
	origH := b.Dy()
	n := d.inputSize

	pixels, crop := PreprocessCrop(img, n)
	data := d.input.GetData()
	if len(data) != len(pixels) {
		return nil, fmt.Errorf("tensor size mismatch: got %d want %d", len(data), len(pixels))
	}
	copy(data, pixels)

	if err := d.session.Run(); err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	outData := d.output.GetData()
	expectedOut := 1 * n * n
	if len(outData) < expectedOut {
		return nil, fmt.Errorf("unexpected output size: got %d want >=%d", len(outData), expectedOut)
	}

	return DepthToColorCrop(outData, n, crop, origW, origH), nil
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
	// ONNX Runtime 环境是进程级单例：首个 Detector 通过 ortEnvOnce 初始化，
	// 共享给所有并行 Detector。这里不销毁环境，避免提前释放导致其他
	// session 崩溃。
}

// DefaultLibPath returns the path to the ONNX Runtime shared library.
// Delegates to onnxrt.LibPath for centralized logic.
func DefaultLibPath(modelsDir string) (string, error) {
	return onnxrt.LibPath(modelsDir)
}

// DefaultModelPath 返回默认的 Depth Anything V2 模型路径。
func DefaultModelPath(modelsDir string) string {
	return filepath.Join(modelsDir, "depth", "depth-anything-v2-small.onnx")
}
