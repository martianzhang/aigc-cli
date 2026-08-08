package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/depth"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// depth 命令的 flag 变量（图片 + 视频共用一套）。
var (
	depthInput      string
	depthOutput     string
	depthModel      string
	depthSize       int
	depthInvert     bool
	depthColor      bool
	depthDryRun     bool
	depthStart      string
	depthEnd        string
	depthKeepAudio  bool
	depthEncodeArgs string
	depthNoSmooth   bool
	depthParallel   int
	depthPreview    bool
)

// imageExts 是 depth 命令识别为图片输入的后缀集合。
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true,
	".gif": true, ".avif": true, ".heic": true, ".jxl": true,
}

// depthCmd 把图片或视频转换为灰度深度图/视频（近亮远暗），本地 ONNX 推理。
var depthCmd = &cobra.Command{
	Use:          "depth",
	Short:        "Convert image/video to grayscale depth map (offline ONNX)",
	SilenceUsage: true,
	Long: `Convert an image or video into a grayscale depth map (near = white, far = black)
using a local Depth Anything ONNX model.

Completely offline — no API key needed. The output is the standard input for
depth-guided / control-video image-to-video workflows (Wan VACE, Kling Motion
Control, Vidu Reference-to-Video): upload the depth map as a motion/space
reference plus a reference photo, and the model generates new content keeping
the original structure with a new appearance.

Input type is auto-detected by extension:
  image  (.png/.jpg/.jpeg/.webp/.bmp/...) → <name>_depth.png
  video  (.mp4/.mov/.mkv/...)            → <name>_depth.mp4 (requires ffmpeg)

Prerequisite: Run "aigc-cli depth init" to download the ONNX Runtime + model.

Examples:
  aigc-cli depth -i photo.jpg
  aigc-cli depth -i photo.jpg --invert
  aigc-cli depth -i video.mp4 --start-time 00:01:00 --end-time 00:01:30
  aigc-cli depth -i video.mp4 --size 378
  aigc-cli depth -i photo.jpg --dry-run`,
	RunE: runDepth,
}

// runDepth 根据输入文件类型路由到图片或视频深度转换。
func runDepth(cmd *cobra.Command, args []string) error {
	input := depthInput
	if input == "" {
		return fmt.Errorf("input required: use --input/-i <file>")
	}
	if _, err := filepath.Abs(input); err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}

	if isImageInput(input) {
		return runDepthImage(cmd)
	}
	return runDepthVideo(cmd)
}

// isImageInput 按扩展名判断输入是否为图片。
func isImageInput(path string) bool {
	return imageExts[strings.ToLower(filepath.Ext(path))]
}

// runDepthImage 处理图片输入 → 灰度深度图 PNG。
func runDepthImage(cmd *cobra.Command) error {
	modelInfo, ok := depth.ResolveModel(depthModel)
	if !ok {
		modelInfo, _ = depth.ResolveModel(depth.DefaultModelID)
	}
	outPath := depthOutput
	if outPath == "" {
		stem := strings.TrimSuffix(filepath.Base(depthInput), filepath.Ext(depthInput))
		outPath = filepath.Join(shared.OutputDir, stem+"_depth.png")
	}

	if depthDryRun {
		size := depthSize
		if size == 0 {
			size = depth.ModelInputSize
		}
		fmt.Printf("# Depth conversion dry run\n")
		fmt.Printf("# input:  %s\n", depthInput)
		fmt.Printf("# output: %s\n", outPath)
		fmt.Printf("# model:  %s (%s)\n", modelInfo.ID, modelInfo.Desc)
		fmt.Printf("# size:   %d (short side, 14-aligned)\n", size)
		fmt.Printf("# invert: %v, color: %v\n", depthInvert, depthColor)
		fmt.Printf("# (single-image inference, no ffmpeg needed)\n")
		return nil
	}

	out, err := depth.ConvertImage(depth.ImageOptions{
		Input:         depthInput,
		Output:        outPath,
		ModelID:       depthModel,
		InferenceSize: depthSize,
		Invert:        depthInvert,
		Color:         depthColor,
		Verbose:       shared.Verbose,
	})
	if err != nil {
		return err
	}
	fmt.Printf("Depth image saved: %s\n", out)
	if depthPreview {
		if err := service.PreviewFile(out); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", err)
		}
	}
	return nil
}

// runDepthVideo 处理视频输入 → 灰度深度视频（复用 internal/depth.Convert）。
func runDepthVideo(cmd *cobra.Command) error {
	modelInfo, ok := depth.ResolveModel(depthModel)
	if !ok {
		modelInfo, _ = depth.ResolveModel(depth.DefaultModelID)
	}
	outPath := depthOutput
	if outPath == "" {
		stem := strings.TrimSuffix(filepath.Base(depthInput), filepath.Ext(depthInput))
		outPath = filepath.Join(shared.OutputDir, stem+"_depth.mp4")
	}

	if depthDryRun {
		printDepthDryRun(depthDryRunInfo{
			input:      depthInput,
			outPath:    outPath,
			model:      modelInfo,
			startTime:  depthStart,
			endTime:    depthEnd,
			invert:     depthInvert,
			color:      depthColor,
			parallel:   depthParallel,
			noSmooth:   depthNoSmooth,
			keepAudio:  depthKeepAudio,
			encodeArgs: depthEncodeArgs,
		})
		return nil
	}

	out, err := depth.Convert(depth.ConvertOptions{
		Input:         depthInput,
		Output:        outPath,
		ModelID:       depthModel,
		InferenceSize: depthSize,
		StartTime:     depthStart,
		EndTime:       depthEnd,
		Invert:        depthInvert,
		Color:         depthColor,
		Parallel:      depthParallel,
		Smooth:        !depthNoSmooth,
		KeepAudio:     depthKeepAudio,
		EncodeArgs:    depthEncodeArgs,
		Verbose:       shared.Verbose,
		OnProgress: func(done, total int, fps float64) {
			fmt.Printf("  frame %d/%d (%.1f fps)\n", done, total, fps)
		},
	})
	if err != nil {
		return err
	}
	fmt.Printf("\nDepth video saved: %s\n", out)
	if depthPreview {
		if err := service.PreviewFile(out); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", err)
		}
	}
	return nil
}

func init() {
	registerDepthFlags(depthCmd)
	rootCmd.AddCommand(depthCmd)
}

// registerDepthFlags 注册 depth 命令的 flag。
func registerDepthFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&depthInput, "input", "i", "", "Input image or video file")
	f.StringVarP(&depthOutput, "output", "o", "", "Output path (default: <name>_depth.png/.mp4)")
	f.StringVar(&depthModel, "model", "", fmt.Sprintf("Depth model (default: %s). Options: %s", depth.DefaultModelID, strings.Join(depth.ListModelIDs(), ", ")))
	f.IntVar(&depthSize, "size", 0, "Inference resolution, short side (14-aligned; default 280 video / 518 image)")
	f.BoolVar(&depthInvert, "invert", false, "Invert depth (near = black instead of near = white)")
	f.BoolVar(&depthColor, "color", false, "Output a Spectral_r colored depth map (near = warm red/orange, far = cool blue/purple)")
	f.BoolVar(&depthDryRun, "dry-run", false, "Print what would run without doing it")
	f.StringVar(&depthStart, "start-time", "", "Video: start time (SS, MM:SS, HH:MM:SS)")
	f.StringVar(&depthEnd, "end-time", "", "Video: end time; alone = convert the first N seconds")
	f.BoolVar(&depthKeepAudio, "keep-audio", false, "Video: keep the source audio track")
	f.StringVar(&depthEncodeArgs, "encode-args", "", "Video: extra ffmpeg encode args appended after defaults (same-named options override, e.g. \"-crf 28 -preset slow\")")
	f.BoolVar(&depthNoSmooth, "no-smooth", false, "Video: disable temporal smoothing (reduces flicker)")
	f.IntVarP(&depthParallel, "parallel", "p", 0, "Video: number of parallel inference workers (default: auto by CPU cores)")
	f.BoolVar(&depthPreview, "preview", false, "Open the depth result with the system default viewer")
}

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
