package cmd

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/depth"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

// depth conversion flag variables (registered on videoCmd).
var (
	vidDepthConvert   bool
	vidDepthInput     string
	vidDepthStart     string
	vidDepthEnd       string
	vidDepthModel     string
	vidDepthSize      int
	vidDepthInvert    bool
	vidDepthNoSmooth  bool
	vidDepthKeepAudio bool
)

const (
	depthTmpFramePrefix = "depth_frame_"
	depthTemporalBlend  = 0.45 // how strongly prev frame blends into current (graygar TEMPORAL_BLEND)
	depthRangeEMA       = 0.90 // EMA factor for normalization range across frames (graygar RANGE_EMA)
)

// runVideoDepth 处理 video --convert-to-depth：把普通视频转为灰度深度视频。
// 通过 ffmpeg 子进程抽帧/编码（纯 Go 无 CGO 生态下的事实标准方案）。
func runVideoDepth(cmd *cobra.Command) error {
	input := vidDepthInput
	if input == "" {
		return fmt.Errorf("input video required: use --input/-i <file>")
	}
	if _, err := os.Stat(input); err != nil {
		return fmt.Errorf("input video not found: %w", err)
	}

	// Resolve model (default: depth-anything-v2-small, Apache-2.0)
	modelID := vidDepthModel
	if modelID == "" {
		modelID = depth.DefaultModelID
	}
	modelInfo, ok := depth.ResolveModel(modelID)
	if !ok {
		return fmt.Errorf("unknown depth model %q (available: %s, or aliases: small, base, large)",
			modelID, strings.Join(depth.ListModelIDs(), ", "))
	}

	// Output path: <output_dir>/<input_stem>_depth.mp4
	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	outPath := filepath.Join(shared.OutputDir, stem+"_depth.mp4")

	// ffmpeg availability check
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return ffmpegMissingHint()
	}

	// Build extraction command (probe first for fps/resolution)
	ffprobe := ffprobeAvailable()
	videoInfo, err := probeVideo(input, ffprobe)
	if err != nil {
		return fmt.Errorf("probe video: %w", err)
	}
	if videoInfo.FPS <= 0 {
		videoInfo.FPS = 30
	}

	if vidDryRun {
		printDepthDryRun(input, outPath, videoInfo, modelInfo)
		return nil
	}

	fmt.Printf("Converting %s → %s\n", input, outPath)
	fmt.Printf("  model=%s (%s)  fps=%.2f  %dx%d  invert=%v  smooth=%v\n",
		modelInfo.ID, modelInfo.License, videoInfo.FPS, videoInfo.Width, videoInfo.Height, vidDepthInvert, !vidDepthNoSmooth)

	// Init ONNX Runtime + depth detector
	modelsDir := filepath.Join(configDir(), "models")
	if _, err := onnxrt.EnsureInstalled(modelsDir, false); err != nil {
		return fmt.Errorf("ONNX Runtime install: %w", err)
	}
	libPath, err := depth.DefaultLibPath(modelsDir)
	if err != nil {
		return err
	}
	modelPath := depth.ModelPath(modelsDir, modelID)
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("depth model not found at %s\n  Run: aigc-cli video init", modelPath)
	}
	// Inference resolution: --depth-size N (short side, 14-aligned).
	// Default 378 (~2× faster than official 518, quality sufficient for
	// motion-reference depth videos); use --depth-size 518 for max quality.
	infSize := vidDepthSize
	if infSize == 0 {
		infSize = depth.DefaultInferenceSize
	}
	det, err := depth.NewDetectorWithSize(libPath, modelPath, infSize)
	if err != nil {
		return fmt.Errorf("init depth detector: %w", err)
	}
	defer det.Close()

	// Extract frames to temp dir
	tmpDir, err := os.MkdirTemp("", "aigc-depth-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractFrames(input, tmpDir, videoInfo); err != nil {
		return err
	}

	// Process each frame
	frames, err := filepath.Glob(filepath.Join(tmpDir, depthTmpFramePrefix+"*"))
	if err != nil || len(frames) == 0 {
		return fmt.Errorf("no frames extracted")
	}

	smooth := !vidDepthNoSmooth
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
			return fmt.Errorf("depth inference on %s: %w", framePath, err)
		}

		// Temporal smoothing (blend with prev frame) + EMA normalization range
		cur := gray.Pix
		if smooth && prevGray != nil {
			blended := make([]uint8, len(cur))
			for j := range cur {
				blended[j] = uint8(float32(depthTemporalBlend)*float32(prevGray[j]) +
					(1-depthTemporalBlend)*float32(cur[j]))
			}
			cur = blended
		}
		prevGray = cur

		if smooth && emaLo >= 0 {
			lo, hi := percentileRange(cur, 1.0, 99.0)
			emaLo = depthRangeEMA*emaLo + (1-depthRangeEMA)*lo
			emaHi = depthRangeEMA*emaHi + (1-depthRangeEMA)*hi
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
			if vidDepthInvert {
				n = 1 - n
			}
			normed[j] = uint8(n * 255)
		}
		if err := writeGrayPNG(framePath, normed, gray.Bounds().Dx(), gray.Bounds().Dy()); err != nil {
			return err
		}

		if i%10 == 0 || i == len(frames)-1 {
			elapsed := time.Since(t0)
			fmt.Printf("  frame %d/%d (%.1f fps)\n", i+1, len(frames), float64(i+1)/elapsed.Seconds())
		}
	}

	// Encode depth frames into H.264 mp4
	startSec, err := parseTimeToSeconds(vidDepthStart)
	if err != nil {
		return err
	}
	endSec, err := parseTimeToSeconds(vidDepthEnd)
	if err != nil {
		return err
	}
	durSec := 0.0
	if endSec > 0 {
		durSec = endSec - startSec
		if durSec < 0 {
			durSec = 0
		}
	}
	if err := encodeDepthVideo(tmpDir, outPath, input, videoInfo, startSec, durSec); err != nil {
		return err
	}

	fmt.Printf("\nDepth video saved: %s\n", outPath)
	return nil
}

// videoInfo 保存抽帧/编码所需的视频元信息。
type videoInfo struct {
	FPS    float64
	Width  int
	Height int
}

// ffprobeAvailable reports whether ffprobe is on PATH.
func ffprobeAvailable() bool {
	_, err := exec.LookPath("ffprobe")
	return err == nil
}

// probeVideo 用 ffprobe 读取视频元信息；无 ffprobe 时返回保守默认值。
func probeVideo(input string, haveFFprobe bool) (*videoInfo, error) {
	info := &videoInfo{FPS: 30}
	if !haveFFprobe {
		fmt.Println("  (ffprobe not found; assuming 30 fps)")
		return info, nil
	}
	out, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate",
		"-of", "csv=p=0", input).Output()
	if err != nil {
		return info, fmt.Errorf("ffprobe failed (is the file a valid video?): %w", err)
	}
	fields := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(fields) >= 3 {
		fmt.Sscanf(fields[0], "%d", &info.Width)
		fmt.Sscanf(fields[1], "%d", &info.Height)
		var num, den int
		n, _ := fmt.Sscanf(fields[2], "%d/%d", &num, &den)
		if n == 2 && den > 0 {
			info.FPS = float64(num) / float64(den)
		}
	}
	return info, nil
}

// parseTimeToSeconds 把时间字符串（SS、MM:SS、HH:MM:SS）解析为秒。
func parseTimeToSeconds(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	parts := strings.Split(s, ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("invalid time %q (expected SS, MM:SS, or HH:MM:SS)", s)
	}
	var total float64
	mult := 1.0
	for i := len(parts) - 1; i >= 0; i-- {
		v, err := strconv.ParseFloat(parts[i], 64)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("invalid time %q (expected SS, MM:SS, or HH:MM:SS)", s)
		}
		total += v * mult
		mult *= 60
	}
	return total, nil
}

// extractFrames 用 ffmpeg 从视频抽取帧为 PNG，支持 --start-time/--end-time。
func extractFrames(input, tmpDir string, info *videoInfo) error {
	pattern := filepath.Join(tmpDir, depthTmpFramePrefix+"%06d.png")
	args := []string{"-y", "-i", input}
	if vidDepthStart != "" {
		args = append(args, "-ss", vidDepthStart)
	}
	if vidDepthEnd != "" {
		args = append(args, "-to", vidDepthEnd)
	}
	args = append(args, "-vf", fmt.Sprintf("fps=%.3f", info.FPS))
	args = append(args, pattern)

	if shared.Verbose {
		fmt.Printf("  ffmpeg %s\n", strings.Join(args, " "))
	}
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg frame extraction failed: %w\n%s", err, string(out))
	}
	return nil
}

// encodeDepthVideo 把灰度帧序列编码为 H.264 mp4（yuv420p + faststart）。
// 可选：第二遍 ffmpeg 把源视频音轨 mux 进输出，音轨按 startSec/durSec 对齐截取。
func encodeDepthVideo(tmpDir, outPath, inputPath string, info *videoInfo, startSec, durSec float64) error {
	pattern := filepath.Join(tmpDir, depthTmpFramePrefix+"%06d.png")
	args := []string{"-y", "-framerate", fmt.Sprintf("%.3f", info.FPS), "-i", pattern,
		"-c:v", "libx264", "-preset", "medium", "-crf", "18",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart"}
	args = append(args, outPath)

	if shared.Verbose {
		fmt.Printf("  ffmpeg %s\n", strings.Join(args, " "))
	}
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg encode failed: %w\n%s", err, string(out))
	}

	if !vidDepthKeepAudio {
		return nil
	}
	// Second pass: mux the source audio track aligned to the conversion range.
	// -ss/-t are placed before -i inputPath so they seek/limit the audio input
	// only (the first input is the already-encoded depth video).
	muxArgs := []string{"-y", "-i", outPath}
	if startSec > 0 {
		muxArgs = append(muxArgs, "-ss", fmt.Sprintf("%.3f", startSec))
	}
	if durSec > 0 {
		muxArgs = append(muxArgs, "-t", fmt.Sprintf("%.3f", durSec))
	}
	muxArgs = append(muxArgs, "-i", inputPath,
		"-map", "0:v:0", "-map", "1:a:0?", "-c", "copy", "-shortest", outPath+".tmp.mp4")
	if out, err := exec.Command("ffmpeg", muxArgs...).CombinedOutput(); err == nil {
		os.Rename(outPath+".tmp.mp4", outPath)
	} else if shared.Verbose {
		fmt.Fprintf(os.Stderr, "Warning: audio mux failed (source may have no audio): %v\n%s", err, string(out))
	}
	return nil
}

// loadFrame 从 PNG 文件加载一帧。
func loadFrame(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// writeGrayPNG 以灰度 PNG 覆盖写入帧文件（复用抽帧路径，避免额外清理）。
func writeGrayPNG(path string, pix []uint8, w, h int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return depth.SaveGrayPNG(f, pix, w, h)
}

// percentileRange 返回灰度数据的 lo/hi 百分位（用于归一化范围）。
// 语义：从低端跳过 loPct% 的元素得到 lo，从高端跳过 (100-hiPct)% 的元素得到 hi。
func percentileRange(data []uint8, loPct, hiPct float64) (float32, float32) {
	if len(data) == 0 {
		return 0, 255
	}
	hist := make([]int, 256)
	for _, v := range data {
		hist[v]++
	}
	total := len(data)
	loSkip := int(float64(total) * loPct / 100.0)
	hiSkip := int(float64(total) * (100.0 - hiPct) / 100.0)

	lo := 0
	cum := 0
	for i := 0; i < 256; i++ {
		cum += hist[i]
		if cum > loSkip {
			lo = i
			break
		}
	}
	hi := 255
	cum = 0
	for i := 255; i >= 0; i-- {
		cum += hist[i]
		if cum > hiSkip {
			hi = i
			break
		}
	}
	return float32(lo), float32(hi)
}

// printDepthDryRun 打印将要执行的 ffmpeg 命令（--dry-run）。
func printDepthDryRun(input, outPath string, info *videoInfo, model depth.ModelInfo) {
	fmt.Printf("# Depth conversion dry run\n")
	fmt.Printf("# input:  %s\n", input)
	fmt.Printf("# output: %s\n", outPath)
	fmt.Printf("# model:  %s (%s)\n", model.ID, model.Desc)
	fmt.Printf("# fps:    %.2f, size: %dx%d, invert: %v, smooth: %v\n",
		info.FPS, info.Width, info.Height, vidDepthInvert, !vidDepthNoSmooth)

	tmp := "/tmp/aigc-depth-frames"
	pattern := filepath.Join(tmp, depthTmpFramePrefix+"%06d.png")

	extract := []string{"ffmpeg", "-y", "-i", input}
	if vidDepthStart != "" {
		extract = append(extract, "-ss", vidDepthStart)
	}
	if vidDepthEnd != "" {
		extract = append(extract, "-to", vidDepthEnd)
	}
	extract = append(extract, "-vf", fmt.Sprintf("fps=%.3f", info.FPS), pattern)
	fmt.Printf("%s\n", strings.Join(extract, " "))

	encode := []string{"ffmpeg", "-y", "-framerate", fmt.Sprintf("%.3f", info.FPS),
		"-i", pattern, "-c:v", "libx264", "-preset", "medium", "-crf", "18",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart", outPath}
	fmt.Printf("%s\n", strings.Join(encode, " "))

	if vidDepthKeepAudio {
		startSec, _ := parseTimeToSeconds(vidDepthStart)
		endSec, _ := parseTimeToSeconds(vidDepthEnd)
		durSec := 0.0
		if endSec > 0 {
			durSec = endSec - startSec
			if durSec < 0 {
				durSec = 0
			}
		}
		mux := []string{"ffmpeg", "-y", "-i", outPath}
		if startSec > 0 {
			mux = append(mux, "-ss", fmt.Sprintf("%.3f", startSec))
		}
		if durSec > 0 {
			mux = append(mux, "-t", fmt.Sprintf("%.3f", durSec))
		}
		mux = append(mux, "-i", input,
			"-map", "0:v:0", "-map", "1:a:0?", "-c", "copy", "-shortest", outPath+".tmp.mp4")
		fmt.Printf("%s\n", strings.Join(mux, " "))
	}

	fmt.Printf("\n# Then run per-frame depth inference between the two commands.\n")
}

// ffmpegMissingHint 打印各平台 ffmpeg 安装建议（不自动安装）。
func ffmpegMissingHint() error {
	return fmt.Errorf(`ffmpeg not found in PATH.

Depth conversion requires ffmpeg for frame extraction and encoding.
Install it on your platform (ffmpeg itself is NOT bundled with aigc-cli):

  macOS:    brew install ffmpeg
  Ubuntu:   sudo apt install ffmpeg
  Debian:   sudo apt-get install ffmpeg
  Fedora:   sudo dnf install ffmpeg
  Arch:     sudo pacman -S ffmpeg
  Windows:  winget install Gyan.FFmpeg
            # or: choco install ffmpeg

Verify with: ffmpeg -version`)
}
