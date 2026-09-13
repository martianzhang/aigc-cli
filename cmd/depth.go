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
	depthSkeleton   bool
	depthFace       bool
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

// runDepth 根据输入文件类型路由到深度转换或标注模式。
func runDepth(cmd *cobra.Command, args []string) error {
	input := depthInput
	if input == "" {
		return fmt.Errorf("input required: use --input/-i <file>")
	}
	if _, err := filepath.Abs(input); err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}

	if depthSkeleton || depthFace {
		if isImageInput(input) {
			return runDepthAnnotateImage(cmd)
		}
		return runDepthAnnotateVideo(cmd)
	}

	if isImageInput(input) {
		return runDepthImage(cmd)
	}
	return runDepthVideo(cmd)
}

// runDepthAnnotateImage 先求深度图，再在深度图上叠加骨架/人脸标注。
func runDepthAnnotateImage(cmd *cobra.Command) error {
	// 先做深度转换（输出 _depth.png）
	if err := runDepthImage(cmd); err != nil {
		return err
	}

	// 在深度图上叠加标注
	if depthSkeleton {
		if err := annotateSkeleton(depthInput, depthAnnotatePath()); err != nil {
			return err
		}
	}
	if depthFace {
		if err := annotateFace(depthInput, depthAnnotatePath()); err != nil {
			return err
		}
	}

	if depthPreview {
		if err := service.PreviewFile(depthAnnotatePath()); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", err)
		}
	}
	return nil
}

// depthAnnotatePath 计算标注输出的路径（覆盖深度图，_depth 后缀）。
func depthAnnotatePath() string {
	if depthOutput != "" {
		return depthOutput
	}
	stem := strings.TrimSuffix(filepath.Base(depthInput), filepath.Ext(depthInput))
	return filepath.Join(shared.OutputDir, stem+"_depth.png")
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
	// 标注模式下 preview 延后到叠加完成后统一执行（避免两次 preview）
	if depthPreview && !depthSkeleton && !depthFace {
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
	f.BoolVar(&depthSkeleton, "skeleton", false, "Detect human pose and draw COCO17 skeleton on the depth output (image or video)")
	f.BoolVar(&depthFace, "face", false, "Detect faces, draw landmarks and eyes on the depth output (image or video)")
}
