package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/depth"
)

// depth conversion flag variables (registered on videoCmd).
var (
	vidDepthConvert    bool
	vidDepthInput      string
	vidDepthStart      string
	vidDepthEnd        string
	vidDepthModel      string
	vidDepthSize       int
	vidDepthInvert     bool
	vidDepthNoSmooth   bool
	vidDepthKeepAudio  bool
	vidDepthEncodeArgs string
)

// runVideoDepth 处理 video --convert-to-depth：把普通视频转为灰度深度视频。
// 核心管线在 internal/depth.Convert（cmd 与 MCP/chat 工具复用）。
func runVideoDepth(cmd *cobra.Command) error {
	input := vidDepthInput
	if input == "" {
		return fmt.Errorf("input video required: use --input/-i <file>")
	}

	modelInfo, ok := depth.ResolveModel(vidDepthModel)
	if !ok {
		modelInfo, _ = depth.ResolveModel(depth.DefaultModelID)
	}
	outPath := filepath.Join(shared.OutputDir, strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))+"_depth.mp4")

	if vidDryRun {
		printDepthDryRun(input, outPath, modelInfo)
		return nil
	}

	out, err := depth.Convert(depth.ConvertOptions{
		Input:         input,
		Output:        outPath,
		ModelID:       vidDepthModel,
		InferenceSize: vidDepthSize,
		StartTime:     vidDepthStart,
		EndTime:       vidDepthEnd,
		Invert:        vidDepthInvert,
		Smooth:        !vidDepthNoSmooth,
		KeepAudio:     vidDepthKeepAudio,
		EncodeArgs:    vidDepthEncodeArgs,
		Verbose:       shared.Verbose,
		OnProgress: func(done, total int, fps float64) {
			fmt.Printf("  frame %d/%d (%.1f fps)\n", done, total, fps)
		},
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nDepth video saved: %s\n", out)
	return nil
}

// printDepthDryRun 打印将要执行的 ffmpeg 命令（--dry-run）。
func printDepthDryRun(input, outPath string, model depth.ModelInfo) {
	fmt.Printf("# Depth conversion dry run\n")
	fmt.Printf("# input:  %s\n", input)
	fmt.Printf("# output: %s\n", outPath)
	fmt.Printf("# model:  %s (%s)\n", model.ID, model.Desc)
	fmt.Printf("# invert: %v, smooth: %v\n", vidDepthInvert, !vidDepthNoSmooth)

	tmp := "/tmp/aigc-depth-frames"
	pattern := filepath.Join(tmp, "depth_frame_%06d.png")

	extract := []string{"ffmpeg", "-y", "-i", input}
	if vidDepthStart != "" {
		extract = append(extract, "-ss", vidDepthStart)
	}
	if vidDepthEnd != "" {
		extract = append(extract, "-to", vidDepthEnd)
	}
	extract = append(extract, "-vf", "fps=24", pattern)
	fmt.Printf("%s\n", strings.Join(extract, " "))

	encode := []string{"ffmpeg", "-y", "-framerate", "24",
		"-i", pattern, "-c:v", "libx264", "-preset", "medium", "-crf", "18",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart", outPath}
	fmt.Printf("%s\n", strings.Join(encode, " "))

	fmt.Printf("\n# Then run per-frame depth inference between the two commands.\n")
}
