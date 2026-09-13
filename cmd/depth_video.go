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

// runDepthAnnotateVideo 先求深度视频，再逐帧叠加骨架/人脸标注（复用 Convert 的 Annotate 回调）。
func runDepthAnnotateVideo(cmd *cobra.Command) error {
	outPath := depthOutput
	if outPath == "" {
		stem := strings.TrimSuffix(filepath.Base(depthInput), filepath.Ext(depthInput))
		outPath = filepath.Join(shared.OutputDir, stem+"_depth.mp4")
	}

	annotate, closeDetectors, err := newAnnotateVideoCallback(annotateVideoOptions{
		skeleton: depthSkeleton,
		face:     depthFace,
	})
	if err != nil {
		return err
	}
	defer closeDetectors()

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
		Annotate:      annotate,
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
