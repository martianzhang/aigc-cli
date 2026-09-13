package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/depth"
)

// depthDryRunInfo 携带视频 dry-run 打印所需的全部参数。
type depthDryRunInfo struct {
	input      string
	outPath    string
	model      depth.ModelInfo
	startTime  string
	endTime    string
	invert     bool
	color      bool
	parallel   int
	noSmooth   bool
	keepAudio  bool
	encodeArgs string
}

// printDepthDryRun 打印视频深度转换将要执行的 ffmpeg 命令（--dry-run）。
func printDepthDryRun(info depthDryRunInfo) {
	fmt.Printf("# Depth conversion dry run\n")
	fmt.Printf("# input:  %s\n", info.input)
	fmt.Printf("# output: %s\n", info.outPath)
	fmt.Printf("# model:  %s (%s)\n", info.model.ID, info.model.Desc)
	fmt.Printf("# invert: %v, smooth: %v, keep_audio: %v\n", info.invert, !info.noSmooth, info.keepAudio)
	if info.color {
		fmt.Printf("# color: Spectral_r (near = warm, far = cool)\n")
	}
	if info.parallel > 0 {
		fmt.Printf("# parallel: %d inference workers\n", info.parallel)
	}

	tmp := "/tmp/aigc-depth-frames"
	pattern := filepath.Join(tmp, "depth_frame_%06d.png")

	extract := []string{"ffmpeg", "-y", "-i", info.input}
	if info.startTime != "" {
		extract = append(extract, "-ss", info.startTime)
	}
	if info.endTime != "" {
		extract = append(extract, "-to", info.endTime)
	}
	extract = append(extract, "-vf", "fps=24", pattern)
	fmt.Printf("%s\n", strings.Join(extract, " "))

	encode := []string{"ffmpeg", "-y", "-framerate", "24",
		"-i", pattern, "-c:v", "libx264", "-preset", "medium", "-crf", "23",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart", info.outPath}
	if extra := splitArgs(info.encodeArgs); len(extra) > 0 {
		encode = append(encode[:len(encode)-1], append(extra, info.outPath)...)
	}
	fmt.Printf("%s\n", strings.Join(encode, " "))

	if info.keepAudio {
		fmt.Printf("# audio: source track will be muxed (aligned to start/end)\n")
	}
	fmt.Printf("\n# Then run per-frame depth inference between the two commands.\n")
}

// splitArgs 把空格分隔的参数字符串拆分为切片，支持双引号包裹的值。
// 仅用于 dry-run 打印（实际拆分在 internal/depth.parseEncodeArgs）。
func splitArgs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
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
