package depth

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	// Color 输出 Spectral_r 彩色深度视频（近处暖色/远处冷色）。
	Color bool
	// Parallel 帧级并行推理的 Detector 数量（0 = 自动按机器核数，
	// 默认 min(性能核, 4)）。每个 Detector 使用 optimalThreads()/Parallel
	// 线程，避免过订阅。用户可用 --parallel/-p 覆盖。
	Parallel int
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
	// Create N parallel detectors for frame-level parallelism.
	// Each detector gets optimalThreads()/N threads to avoid oversubscription.
	parallel := opts.Parallel
	if parallel <= 0 {
		parallel = parallelismCount()
	}
	threadsPer := optimalThreads() / parallel
	if threadsPer < 1 {
		threadsPer = 1
	}
	dets := make([]*Detector, parallel)
	for i := range dets {
		d, err := NewDetectorWithSizeThreads(libPath, modelPath, infSize, threadsPer)
		if err != nil {
			closeDetectors(dets)
			return "", fmt.Errorf("init depth detector %d: %w", i, err)
		}
		dets[i] = d
	}
	defer closeDetectors(dets)

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

	// ── Parallel inference + serial smoothing pipeline ──
	// Frame inference is CPU-bound and already fills all cores via intra-op
	// threads. Splitting inference across N detectors (each with fewer
	// threads) increases throughput by avoiding thread oversubscription;
	// smoothing/EMA must stay serial (depends on the previous frame).
	n := len(frames)
	if parallel > n {
		parallel = n
	}
	if parallel < 1 {
		parallel = 1
	}

	// jobChan delivers (frameIndex, framePath) to workers; resultChan
	// collects (frameIndex, gray) back in completion order.
	type frameJob struct {
		idx  int
		path string
	}
	type frameResult struct {
		idx  int
		gray *image.Gray
		err  error
	}
	jobChan := make(chan frameJob, parallel)
	resultChan := make(chan frameResult, n)

	var wg sync.WaitGroup
	for wi := 0; wi < parallel; wi++ {
		wg.Add(1)
		go func(d *Detector) {
			defer wg.Done()
			for job := range jobChan {
				img, err := loadFrame(job.path)
				if err != nil {
					resultChan <- frameResult{job.idx, nil, err}
					continue
				}
				gray, err := d.Predict(img)
				resultChan <- frameResult{job.idx, gray, err}
			}
		}(dets[wi])
	}

	go func() {
		for i, p := range frames {
			jobChan <- frameJob{i, p}
		}
		close(jobChan)
		wg.Wait()
		close(resultChan)
	}()

	results := make([]*image.Gray, n)
	resultsErr := make([]error, n)
	done := 0
	for r := range resultChan {
		results[r.idx] = r.gray
		resultsErr[r.idx] = r.err
		done++
		if opts.OnProgress != nil && (done%10 == 0 || done == n) {
			elapsed := time.Since(t0)
			opts.OnProgress(done, n, float64(done)/elapsed.Seconds())
		}
	}

	for i, framePath := range frames {
		if resultsErr[i] != nil {
			// Skip the frame entirely: remove the source PNG so it never
			// leaks a color frame into the encoded grayscale video.
			fmt.Fprintf(os.Stderr, "Warning: skip %s: %v\n", framePath, resultsErr[i])
			os.Remove(framePath)
			continue
		}
		gray := results[i]

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
			nrm := (float32(v) - emaLo) / rng
			if nrm < 0 {
				nrm = 0
			} else if nrm > 1 {
				nrm = 1
			}
			if opts.Invert {
				nrm = 1 - nrm
			}
			normed[j] = uint8(nrm * 255)
		}
		if err := writeDepthPNG(framePath, normed, gray.Bounds().Dx(), gray.Bounds().Dy(), opts.Color); err != nil {
			return "", err
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

// closeDetectors 关闭一组 Detector 并释放 ONNX Runtime 资源。
func closeDetectors(dets []*Detector) {
	for _, d := range dets {
		if d != nil {
			d.Close()
		}
	}
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
