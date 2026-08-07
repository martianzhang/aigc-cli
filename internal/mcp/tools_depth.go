package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/depth"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

// newConvertVideoDepthTool 定义 convert_video_depth MCP 工具。
// 把视频转成灰度深度视频（近亮远暗），本地 ONNX 推理，无需 API Key。
func newConvertVideoDepthTool() mcp.Tool {
	return mcp.NewTool("convert_video_depth",
		mcp.WithDescription(`Convert a video into a grayscale depth video (near = white, far = black) using a local Depth Anything V2 ONNX model.

Completely offline — no API key needed. The output is the standard input for
depth-guided / control-video image-to-video workflows (Wan VACE, Kling Motion
Control, Vidu Reference-to-Video): upload the depth video as a motion/space
reference plus a reference photo, and the model generates a new video keeping
the original motion with the new appearance.

Prerequisite: Run "aigc-cli video init" to download the ONNX Runtime + model.
Requires ffmpeg on PATH.

Output: H.264 MP4 saved alongside the input as <filename>_depth.mp4
(or the provided output_path).

Examples:
  convert_video_depth input_path="/path/to/video.mp4"
  convert_video_depth input_path="/path/to/video.mp4" start_time="00:01:00" end_time="00:01:30"
  convert_video_depth input_path="/path/to/video.mp4" depth_size=378 invert=true`),
		mcp.WithString("input_path",
			mcp.Required(),
			mcp.Description("Path to the input video file"),
		),
		mcp.WithString("output_path",
			mcp.Description("Optional output path (default: <input>_depth.mp4)"),
		),
		mcp.WithString("start_time",
			mcp.Description("Start time (SS, MM:SS, or HH:MM:SS)"),
		),
		mcp.WithString("end_time",
			mcp.Description("End time; alone = convert the first N seconds"),
		),
		mcp.WithString("model",
			mcp.Description("Depth model: depth-anything-v2-small (default, Apache-2.0) / -base / -large"),
		),
		mcp.WithNumber("depth_size",
			mcp.Description("Inference resolution, short side (14-aligned; default 280; raise to 378/518 for higher quality)"),
		),
		mcp.WithBoolean("invert",
			mcp.Description("Invert depth (near = black instead of near = white)"),
		),
		mcp.WithBoolean("keep_audio",
			mcp.Description("Keep the source audio track in the depth video"),
		),
	)
}

// convertVideoDepthHandler 处理 convert_video_depth 工具调用。
func convertVideoDepthHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("input_path")
		if err != nil {
			return mcp.NewToolResultError("input_path is required"), nil
		}
		if !filepath.IsAbs(path) {
			abs, err := filepath.Abs(path)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid path: %v", err)), nil
			}
			path = abs
		}
		if _, err := os.Stat(path); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("input file not found: %v", err)), nil
		}

		// Resolve ONNX Runtime + model directory.
		sharedDir := filepath.Join(configDir(), "models")
		os.MkdirAll(sharedDir, 0755)
		libPath, err := onnxrt.LibPath(sharedDir)
		if err != nil || libPath == "" {
			libPath, err = onnxrt.EnsureInstalled(sharedDir, false)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("ONNX Runtime not available: %v\nRun: aigc-cli video init", err)), nil
			}
		}
		modelPath := depth.ModelPath(sharedDir, req.GetString("model", ""))
		if _, err := os.Stat(modelPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("depth model not found: %v\nRun: aigc-cli video init", err)), nil
		}

		// Progress output buffer (MCP returns one result at the end).
		var progress strings.Builder
		output := req.GetString("output_path", "")
		started := fmt.Sprintf("Converting %s → %s (may take minutes on CPU)...\n",
			filepath.Base(path), filepath.Base(defaultOutputPath(path, output)))

		out, err := depth.Convert(depth.ConvertOptions{
			Input:         path,
			Output:        output,
			ModelID:       req.GetString("model", ""),
			InferenceSize: int(req.GetFloat("depth_size", 0)),
			StartTime:     req.GetString("start_time", ""),
			EndTime:       req.GetString("end_time", ""),
			Invert:        req.GetBool("invert", false),
			Smooth:        true,
			KeepAudio:     req.GetBool("keep_audio", false),
			LibPath:       libPath,
			ModelsDir:     sharedDir,
			Verbose:       false,
			OnProgress: func(done, total int, fps float64) {
				if done%20 == 0 || done == total {
					fmt.Fprintf(&progress, "  frame %d/%d (%.1f fps)\n", done, total, fps)
				}
			},
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("conversion failed: %v", err)), nil
		}

		return mcp.NewToolResultText(started + progress.String() + "\nDepth video saved: " + out), nil
	}
}

// defaultOutputPath 返回默认输出路径 (<input>_depth.mp4)。
func defaultOutputPath(input, output string) string {
	if output != "" {
		return output
	}
	ext := filepath.Ext(input)
	return strings.TrimSuffix(input, ext) + "_depth.mp4"
}
