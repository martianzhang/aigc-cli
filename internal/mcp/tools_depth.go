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

// mcpImageExts 是 MCP 深度工具识别为图片输入的扩展名集合（与 cmd/depth.go 对齐）。
var mcpImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true,
	".gif": true, ".avif": true, ".heic": true, ".jxl": true,
}

// newConvertDepthTool 定义 convert_depth MCP 工具。
// 把图片或视频转成灰度深度图/视频（近亮远暗），本地 ONNX 推理，无需 API Key。
func newConvertDepthTool() mcp.Tool {
	return mcp.NewTool("convert_depth",
		mcp.WithDescription(`Convert an image or video into a grayscale depth map (near = white, far = black) using a local Depth Anything V2 ONNX model.

Completely offline — no API key needed. The output is the standard input for
depth-guided / control-video image-to-video workflows (Wan VACE, Kling Motion
Control, Vidu Reference-to-Video): upload the depth map as a motion/space
reference plus a reference photo, and the model generates new content keeping
the original structure with the new appearance.

Input type is auto-detected by extension: image → <name>_depth.png (no ffmpeg
needed), video → <name>_depth.mp4 (requires ffmpeg on PATH).

Prerequisite: Run "aigc-cli depth init" to download the ONNX Runtime + model.

Examples:
  convert_depth input_path="/path/to/photo.jpg"
  convert_depth input_path="/path/to/video.mp4"
  convert_depth input_path="/path/to/video.mp4" start_time="00:01:00" end_time="00:01:30"
  convert_depth input_path="/path/to/photo.jpg" invert=true`),
		mcp.WithString("input_path",
			mcp.Required(),
			mcp.Description("Path to the input image or video file"),
		),
		mcp.WithString("output_path",
			mcp.Description("Optional output path (default: <input>_depth.png/.mp4)"),
		),
		mcp.WithString("start_time",
			mcp.Description("Video: start time (SS, MM:SS, or HH:MM:SS)"),
		),
		mcp.WithString("end_time",
			mcp.Description("Video: end time; alone = convert the first N seconds"),
		),
		mcp.WithString("model",
			mcp.Description("Depth model: depth-anything-v2-small (default, Apache-2.0) / -base / -large"),
		),
		mcp.WithNumber("depth_size",
			mcp.Description("Inference resolution, short side (14-aligned; default 280 video / 518 image; raise to 378/518 for higher quality)"),
		),
		mcp.WithBoolean("invert",
			mcp.Description("Invert depth (near = black instead of near = white)"),
		),
		mcp.WithBoolean("keep_audio",
			mcp.Description("Video: keep the source audio track in the depth video"),
		),
	)
}

// convertDepthHandler 处理 convert_depth 工具调用（图片/视频自动路由）。
func convertDepthHandler(cfg *Config) server.ToolHandlerFunc {
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
				return mcp.NewToolResultError(fmt.Sprintf("ONNX Runtime not available: %v\nRun: aigc-cli depth init", err)), nil
			}
		}
		modelPath := depth.ModelPath(sharedDir, req.GetString("model", ""))
		if _, err := os.Stat(modelPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("depth model not found: %v\nRun: aigc-cli depth init", err)), nil
		}

		if mcpImageExts[strings.ToLower(filepath.Ext(path))] {
			return convertDepthImage(ctx, path, sharedDir, libPath, req)
		}
		return convertDepthVideo(ctx, path, sharedDir, libPath, req)
	}
}

// convertDepthImage 处理图片输入 → 灰度深度图 PNG。
func convertDepthImage(ctx context.Context, path, sharedDir, libPath string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := depth.ConvertImage(depth.ImageOptions{
		Input:         path,
		Output:        req.GetString("output_path", ""),
		ModelID:       req.GetString("model", ""),
		InferenceSize: int(req.GetFloat("depth_size", 0)),
		Invert:        req.GetBool("invert", false),
		LibPath:       libPath,
		ModelsDir:     sharedDir,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("conversion failed: %v", err)), nil
	}
	return mcp.NewToolResultText("Depth image saved: " + out), nil
}

// convertDepthVideo 处理视频输入 → 灰度深度视频（H.264 MP4）。
func convertDepthVideo(ctx context.Context, path, sharedDir, libPath string, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Progress output buffer (MCP returns one result at the end).
	var progress strings.Builder
	output := req.GetString("output_path", "")
	started := fmt.Sprintf("Converting %s → %s (may take minutes on CPU)...\n",
		filepath.Base(path), filepath.Base(defaultDepthOutputPath(path, output)))

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

// defaultDepthOutputPath 返回默认输出路径（<input>_depth.<ext>）。
func defaultDepthOutputPath(input, output string) string {
	if output != "" {
		return output
	}
	ext := filepath.Ext(input)
	return strings.TrimSuffix(input, ext) + "_depth" + ext
}
