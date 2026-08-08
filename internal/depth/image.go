package depth

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// ImageOptions 控制单张图片到灰度深度图的转换参数。
type ImageOptions struct {
	// Input 输入图片路径（必填）。
	Input string
	// Output 输出路径；空则默认 <input_stem>_depth.png。
	Output string
	// ModelID 深度模型 ID（默认 DefaultModelID）。
	ModelID string
	// InferenceSize 推理分辨率短边（默认 ModelInputSize，14 对齐）。
	InferenceSize int
	// Invert 反转深度方向（近暗远亮）。
	Invert bool
	// Verbose 打印推理参数。
	Verbose bool
	// LibPath ONNX Runtime 库路径；空则自动解析。
	LibPath string
	// ModelsDir 模型根目录；空则自动解析。
	ModelsDir string
}

// ConvertImage 把单张图片转成灰度深度图（近亮远暗），返回输出路径。
// 深度模型与 ONNX Runtime 需已下载（aigc-cli depth init）。
func ConvertImage(opts ImageOptions) (string, error) {
	input := opts.Input
	if input == "" {
		return "", fmt.Errorf("input image required")
	}
	if _, err := os.Stat(input); err != nil {
		return "", fmt.Errorf("input image not found: %w", err)
	}

	modelID := opts.ModelID
	if modelID == "" {
		modelID = DefaultModelID
	}
	if _, ok := ResolveModel(modelID); !ok {
		return "", fmt.Errorf("unknown depth model %q (available: %s)", modelID, strings.Join(ListModelIDs(), ", "))
	}

	infSize := opts.InferenceSize
	if infSize == 0 {
		infSize = ModelInputSize
	}

	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	outPath := opts.Output
	if outPath == "" {
		outPath = stem + "_depth.png"
	}

	modelsDir := opts.ModelsDir
	if modelsDir == "" {
		modelsDir = defaultModelsDir()
	}
	libPath := opts.LibPath
	var err error
	if libPath == "" {
		libPath, err = onnxrtLibPath(modelsDir)
		if err != nil {
			return "", err
		}
	}
	modelPath := ModelPath(modelsDir, modelID)
	if _, err := os.Stat(modelPath); err != nil {
		return "", fmt.Errorf("depth model not found at %s\n  Run: aigc-cli depth init", modelPath)
	}

	det, err := NewDetectorWithSize(libPath, modelPath, infSize)
	if err != nil {
		return "", fmt.Errorf("init depth detector: %w", err)
	}
	defer det.Close()

	img, err := loadImageFile(input)
	if err != nil {
		return "", err
	}
	gray, err := det.Predict(img)
	if err != nil {
		return "", fmt.Errorf("depth inference: %w", err)
	}

	if opts.Invert {
		for i, v := range gray.Pix {
			gray.Pix[i] = 255 - v
		}
	}

	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create output: %w", err)
	}
	defer f.Close()
	if err := SaveGrayPNG(f, gray.Pix, gray.Bounds().Dx(), gray.Bounds().Dy()); err != nil {
		return "", fmt.Errorf("write depth image: %w", err)
	}
	return outPath, nil
}

// loadImageFile 解码图片文件（PNG/JPEG/BMP/WebP）。
func loadImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}
