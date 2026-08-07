package depth

import (
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

// ConvertOptions 控制视频到深度视频的转换参数。
type ConvertOptions struct {
	// Input 输入视频路径（必填）。
	Input string
	// Output 输出路径；空则默认 <input_stem>_depth.mp4。
	Output string
	// ModelID 深度模型 ID（默认 DefaultModelID）。
	ModelID string
	// InferenceSize 推理分辨率短边（默认 DefaultInferenceSize）。
	InferenceSize int
	// StartTime 起始时间（SS、MM:SS、HH:MM:SS）。
	StartTime string
	// EndTime 结束时间；单独用 = 转前 N 秒。
	EndTime string
	// Invert 反转深度方向（近暗远亮）。
	Invert bool
	// Smooth 时序平滑（默认 true，减轻闪烁）。
	Smooth bool
	// KeepAudio 保留原视频音轨。
	KeepAudio bool
	// EncodeArgs 追加到 ffmpeg 编码命令的自定义参数（空格分隔的字符串）。
	// 追加在默认参数之后，可覆盖 CRF 等默认值，例如 "-crf 28 -preset slow"。
	EncodeArgs string
	// LibPath ONNX Runtime 库路径；空则自动解析。
	LibPath string
	// ModelsDir 模型根目录；空则自动解析。
	ModelsDir string
	// Verbose 打印 ffmpeg 命令。
	Verbose bool
	// OnProgress 每处理若干帧回调（nil 则不回调）。
	OnProgress func(done, total int, fps float64)
}

// Convert 把视频转成灰度深度视频（近亮远暗）。
// 需要系统 ffmpeg 用于抽帧/编码；深度模型与 ONNX Runtime 需已下载。
// 返回输出视频路径。
func Convert(opts ConvertOptions) (string, error) {
	input := opts.Input
	if input == "" {
		return "", fmt.Errorf("input video required")
	}
	if _, err := os.Stat(input); err != nil {
		return "", fmt.Errorf("input video not found: %w", err)
	}

	modelID := opts.ModelID
	if modelID == "" {
		modelID = DefaultModelID
	}
	modelInfo, ok := ResolveModel(modelID)
	if !ok {
		return "", fmt.Errorf("unknown depth model %q (available: %s)", modelID, strings.Join(ListModelIDs(), ", "))
	}

	infSize := opts.InferenceSize
	if infSize == 0 {
		infSize = DefaultInferenceSize
	}

	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	outPath := opts.Output
	if outPath == "" {
		outPath = stem + "_depth.mp4"
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", ffmpegMissingHint()
	}

	info, err := probeVideo(input, ffprobeAvailable())
	if err != nil {
		return "", err
	}
	if info.FPS <= 0 {
		info.FPS = 30
	}

	if opts.Verbose {
		fmt.Printf("Converting %s → %s\n", input, outPath)
		fmt.Printf("  model=%s  fps=%.2f  %dx%d  invert=%v  smooth=%v\n",
			modelInfo.ID, info.FPS, info.Width, info.Height, opts.Invert, opts.Smooth)
	}

	modelsDir := opts.ModelsDir
	if modelsDir == "" {
		modelsDir = defaultModelsDir()
	}
	libPath := opts.LibPath
	if libPath == "" {
		libPath, err = onnxrtLibPath(modelsDir)
		if err != nil {
			return "", err
		}
	}
	modelPath := ModelPath(modelsDir, modelID)
	if _, err := os.Stat(modelPath); err != nil {
		return "", fmt.Errorf("depth model not found at %s\n  Run: aigc-cli video init", modelPath)
	}
	det, err := NewDetectorWithSize(libPath, modelPath, infSize)
	if err != nil {
		return "", fmt.Errorf("init depth detector: %w", err)
	}
	defer det.Close()

	tmpDir, err := os.MkdirTemp("", "aigc-depth-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractFrames(input, tmpDir, info, opts.StartTime, opts.EndTime, opts.Verbose); err != nil {
		return "", err
	}

	frames, err := filepath.Glob(filepath.Join(tmpDir, tmpFramePrefix+"*"))
	if err != nil || len(frames) == 0 {
		return "", fmt.Errorf("no frames extracted")
	}

	smooth := opts.Smooth
	var prevGray []uint8
	var emaLo, emaHi float32 = -1, -1
	t0 := time.Now()

	for i, framePath := range frames {
		img, err := loadFrame(framePath)
		if err != nil {
			// Skip the frame entirely: remove the source PNG so it never
			// leaks a color frame into the encoded grayscale video.
			fmt.Fprintf(os.Stderr, "Warning: skip %s: %v\n", framePath, err)
			os.Remove(framePath)
			continue
		}
		gray, err := det.Predict(img)
		if err != nil {
			return "", fmt.Errorf("depth inference on %s: %w", framePath, err)
		}

		// Temporal smoothing (blend with prev frame) + EMA normalization range
		cur := gray.Pix
		if smooth && prevGray != nil {
			blended := make([]uint8, len(cur))
			for j := range cur {
				blended[j] = uint8(temporalBlend*float32(prevGray[j]) +
					(1-temporalBlend)*float32(cur[j]))
			}
			cur = blended
		}
		prevGray = cur

		if smooth && emaLo >= 0 {
			lo, hi := percentileRange(cur, 1.0, 99.0)
			emaLo = rangeEMA*emaLo + (1-rangeEMA)*lo
			emaHi = rangeEMA*emaHi + (1-rangeEMA)*hi
		} else {
			emaLo, emaHi = percentileRange(cur, 1.0, 99.0)
		}

		normed := make([]uint8, len(cur))
		rng := emaHi - emaLo
		if rng < 1e-6 {
			rng = 1e-6
		}
		for j, v := range cur {
			n := (float32(v) - emaLo) / rng
			if n < 0 {
				n = 0
			} else if n > 1 {
				n = 1
			}
			if opts.Invert {
				n = 1 - n
			}
			normed[j] = uint8(n * 255)
		}
		if err := writeGrayPNG(framePath, normed, gray.Bounds().Dx(), gray.Bounds().Dy()); err != nil {
			return "", err
		}

		if opts.OnProgress != nil && (i%10 == 0 || i == len(frames)-1) {
			elapsed := time.Since(t0)
			opts.OnProgress(i+1, len(frames), float64(i+1)/elapsed.Seconds())
		}
	}

	startSec, err := parseTimeToSeconds(opts.StartTime)
	if err != nil {
		return "", err
	}
	endSec, err := parseTimeToSeconds(opts.EndTime)
	if err != nil {
		return "", err
	}
	durSec := 0.0
	if endSec > 0 {
		durSec = endSec - startSec
		if durSec < 0 {
			durSec = 0
		}
	}
	if err := encodeDepthVideo(tmpDir, outPath, input, info, startSec, durSec, opts.KeepAudio, opts.Verbose, parseEncodeArgs(opts.EncodeArgs)); err != nil {
		return "", err
	}

	return outPath, nil
}

// defaultModelsDir 返回默认模型根目录 (~/.config/aigc-cli/models)。
func defaultModelsDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".config/aigc-cli/models"
	}
	return filepath.Join(home, ".config", "aigc-cli", "models")
}

// onnxrtLibPath 返回 ONNX Runtime 共享库路径。
func onnxrtLibPath(modelsDir string) (string, error) {
	return onnxrt.LibPath(modelsDir)
}
