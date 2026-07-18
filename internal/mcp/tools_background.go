package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/martianzhang/aigc-cli/internal/background"
	"github.com/martianzhang/aigc-cli/internal/rmbg"
)

// removeBackgroundTool 定义 remove_background MCP 工具。
func newRemoveBackgroundTool() mcp.Tool {
	return mcp.NewTool("remove_background",
		mcp.WithDescription(`Remove image background using RMBG 2.0 AI semantic segmentation.

Completely offline — no API key needed. Uses ONNX Runtime + RMBG 2.0 model.

Prerequisite: Run "aigc-cli background init" to download the model first.

Output: A PNG with transparency channel, saved alongside the original file
as <filename>_removebg.png.

Examples:
  remove_background file_path="/path/to/photo.jpg"
  remove_background file_path="/path/to/photo.png" output_path="/path/to/out.png"
  remove_background file_path="/path/to/photo.jpg" replace_color="#FF0000"`),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("Path to the image file to process"),
		),
		mcp.WithString("output_path",
			mcp.Description("Optional output path (default: <input>_removebg.png or <input>_replaced.png)"),
		),
		mcp.WithString("replace_color",
			mcp.Description("Replace background with this hex color (e.g. \"#FF0000\"). Mutually exclusive with replace_image."),
		),
		mcp.WithString("replace_image",
			mcp.Description("Replace background with this image. Mutually exclusive with replace_color."),
		),
		mcp.WithBoolean("autocrop",
			mcp.Description("Crop to foreground bounding box"),
		),
	)
}

// removeBackgroundHandler 处理 remove_background 工具调用。
func removeBackgroundHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}
		if !filepath.IsAbs(path) {
			abs, err := filepath.Abs(path)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid path: %v", err)), nil
			}
			path = abs
		}

		// 检查文件存在
		if _, err := os.Stat(path); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("file not found: %s", path)), nil
		}

		// 解码图片
		f, err := os.Open(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot open file: %v", err)), nil
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot decode image: %v", err)), nil
		}

		// 初始化 RMBG Detector
		modelsDir := filepath.Join(configDir(), "models")
		libPath, err := rmbg.DefaultLibPath(modelsDir)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("ONNX Runtime not found. Run 'aigc-cli background init' first: %v", err)), nil
		}
		modelPath := rmbg.DefaultModelPath(modelsDir)
		if _, err := os.Stat(modelPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("RMBG model not found. Run 'aigc-cli background init' first: %v", err)), nil
		}
		det, err := rmbg.NewDetector(libPath, modelPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to create RMBG detector: %v", err)), nil
		}
		defer det.Close()

		// 解析参数
		opts := background.Defaults()
		if ac, ok := req.GetArguments()["autocrop"].(bool); ok && ac {
			opts.Autocrop = true
		}

		replaceColor := req.GetString("replace_color", "")
		replaceImage := req.GetString("replace_image", "")
		hasReplace := replaceColor != "" || replaceImage != ""

		// 执行
		var result *background.Result
		var outImg *image.NRGBA

		if hasReplace {
			if replaceColor != "" {
				c, perr := parseHexColorMCP(replaceColor)
				if perr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("invalid replace_color: %v", perr)), nil
				}
				outImg, result, err = background.ReplaceColor(img, c, &opts, det)
			} else {
				rimg, lerr := loadImage(replaceImage)
				if lerr != nil {
					return mcp.NewToolResultError(fmt.Sprintf("cannot load replace_image: %v", lerr)), nil
				}
				outImg, result, err = background.ReplaceImage(img, rimg, &opts, det)
			}
		} else {
			outImg, result, err = background.RemoveBackground(img, &opts, det)
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("background removal failed: %v", err)), nil
		}

		// 确定输出路径
		outputPath := req.GetString("output_path", "")
		if outputPath == "" {
			ext := filepath.Ext(path)
			base := strings.TrimSuffix(filepath.Base(path), ext)
			suffix := "_removebg"
			if hasReplace {
				suffix = "_replaced"
			}
			outputPath = filepath.Join(filepath.Dir(path), base+suffix+".png")
		}

		if err := background.SavePNG(outputPath, outImg); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to save output: %v", err)), nil
		}

		// 返回结果
		info := map[string]interface{}{
			"output_path": outputPath,
			"width":       result.Width,
			"height":      result.Height,
		}
		data, _ := json.MarshalIndent(info, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func configDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".config/aigc-cli"
	}
	return filepath.Join(home, ".config", "aigc-cli")
}

func parseHexColorMCP(s string) (color.Color, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return nil, fmt.Errorf("color must be 6-digit hex, got %q", s)
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b); err != nil {
		return nil, fmt.Errorf("invalid hex color %q", s)
	}
	return color.NRGBA{R: r, G: g, B: b, A: 255}, nil
}

func loadImage(path string) (image.Image, error) {
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		path = abs
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
