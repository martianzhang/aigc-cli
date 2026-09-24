package depth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/depth"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// runDepthAnnotateVideo computes the depth video, then overlays skeleton/face
// marks per frame via Convert's Annotate callback.
func runDepthAnnotateVideo(cmd *cobra.Command) error {
	outPath := depthOutputPath(depthInput, "_depth.mp4")

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
		Verbose:       d.Verbose,
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

// runDepthVideo handles video input → grayscale depth video.
func runDepthVideo(cmd *cobra.Command) error {
	modelInfo, ok := depth.ResolveModel(depthModel)
	if !ok {
		modelInfo, _ = depth.ResolveModel(depth.DefaultModelID)
	}
	outPath := depthOutputPath(depthInput, "_depth.mp4")

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
		Verbose:       d.Verbose,
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

// depthDryRunInfo carries the parameters needed for the video dry-run print.
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

// printDepthDryRun prints the ffmpeg commands the video conversion would run.
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

// splitArgs splits a space-separated argument string, honoring double quotes.
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
