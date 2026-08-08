package depth

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	tmpFramePrefix = "depth_frame_"
	temporalBlend  = 0.45 // how strongly prev frame blends into current
	rangeEMA       = 0.90 // EMA factor for normalization range across frames
)

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

// extractFrames 用 ffmpeg 从视频抽取帧为 PNG，支持 startTime/endTime。
func extractFrames(input, tmpDir string, info *videoInfo, startTime, endTime string, verbose bool) error {
	pattern := filepath.Join(tmpDir, tmpFramePrefix+"%06d.png")
	args := []string{"-y", "-i", input}
	if startTime != "" {
		args = append(args, "-ss", startTime)
	}
	if endTime != "" {
		args = append(args, "-to", endTime)
	}
	args = append(args, "-vf", fmt.Sprintf("fps=%.3f", info.FPS))
	args = append(args, pattern)

	if verbose {
		fmt.Printf("  ffmpeg %s\n", strings.Join(args, " "))
	}
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg frame extraction failed: %w\n%s", err, string(out))
	}
	return nil
}

// parseEncodeArgs 把空格分隔的 ffmpeg 参数字符串拆分为参数切片。
// 支持双引号包裹的值（如 -vf "scale=100:100"）。
func parseEncodeArgs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// 简单拆分：按空格切分，遇到引号包裹时整体作为一个参数。
	var args []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

// encodeDepthVideo 把灰度帧序列编码为 H.264 mp4（yuv420p + faststart）。
// extraArgs 为用户自定义的 ffmpeg 编码参数（追加在默认参数之后，可覆盖 CRF 等）。
// 可选：第二遍 ffmpeg 把源视频音轨 mux 进输出，音轨按 startSec/durSec 对齐截取。
func encodeDepthVideo(tmpDir, outPath, inputPath string, info *videoInfo, startSec, durSec float64, keepAudio, verbose bool, extraArgs []string) error {
	pattern := filepath.Join(tmpDir, tmpFramePrefix+"%06d.png")
	args := []string{"-y", "-framerate", fmt.Sprintf("%.3f", info.FPS), "-i", pattern,
		"-c:v", "libx264", "-preset", "medium", "-crf", "23",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart"}
	args = append(args, extraArgs...)
	args = append(args, outPath)

	if verbose {
		fmt.Printf("  ffmpeg %s\n", strings.Join(args, " "))
	}
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg encode failed: %w\n%s", err, string(out))
	}

	if !keepAudio {
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
	} else if verbose {
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
	return SaveGrayPNG(f, pix, w, h)
}

// writeDepthPNG 以灰度或 Spectral_r 彩色 PNG 覆盖写入帧文件。
func writeDepthPNG(path string, pix []uint8, w, h int, color bool) error {
	if !color {
		return writeGrayPNG(path, pix, w, h)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	rgba := make([]uint8, w*h*4)
	for i, v := range pix {
		if i >= w*h {
			break
		}
		r, g, b := colorize(v)
		rgba[i*4] = r
		rgba[i*4+1] = g
		rgba[i*4+2] = b
		rgba[i*4+3] = 255
	}
	return SaveColorPNG(f, rgba, w, h)
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
