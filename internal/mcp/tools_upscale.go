package mcp

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/image/draw"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	up "github.com/martianzhang/aigc-cli/internal/upscale"
)

// newUpscaleTool defines the upscale MCP tool.
func newUpscaleTool() mcp.Tool {
	return mcp.NewTool("upscale",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(`Upscale an image 2x/4x with a local Real-ESRGAN / Real-CUGAN / Swin2SR ONNX model.

Completely offline — no API key needed. Large images are processed in feathered tiles.
Output is opaque PNG (the models do not produce an alpha channel).

Prerequisite: Run "aigc-cli upscale init" to download the ONNX Runtime + model.

Models (see "aigc-cli upscale init --list" for licenses): realesr-general-x4v3
(default, general 4x), real-esrgan-x4plus, real-esrgan-x4plus-anime-6b,
real-esrgan-animevideov3, real-cugan-2x (anime 2x), swin2sr-lightweight-x2,
swin2sr-realworld-x4.

Examples:
  upscale input_path="/path/to/photo.jpg"
  upscale input_path="/path/to/art.png" model="real-cugan-2x"
  upscale input_path="/path/to/photo.jpg" scale=2`),
		mcp.WithString("input_path",
			mcp.Required(),
			mcp.Description("Path to the input image file"),
		),
		mcp.WithString("output_path",
			mcp.Description("Optional output path (default: <input>_upscaled.png)"),
		),
		mcp.WithString("model",
			mcp.Description("Model id (default: realesr-general-x4v3)"),
		),
		mcp.WithNumber("scale",
			mcp.Description("Output scale factor (default: the model's native scale; other values resample the result)"),
		),
	)
}

// upscaleHandler handles upscale tool calls.
func upscaleHandler(cfg *Config) server.ToolHandlerFunc {
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

		sharedDir := filepath.Join(configDir(), "models")
		os.MkdirAll(sharedDir, 0755)
		libPath, err := onnxrt.LibPath(sharedDir)
		if err != nil || libPath == "" {
			libPath, err = onnxrt.EnsureInstalled(sharedDir, false)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("ONNX Runtime not available: %v\nRun: aigc-cli upscale init", err)), nil
			}
		}

		modelID := req.GetString("model", "")
		info, ok := up.ResolveModel(modelID)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("unknown model %q", modelID)), nil
		}
		modelPath := up.ModelPath(sharedDir, modelID)
		if _, err := os.Stat(modelPath); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("upscale model not found: %v\nRun: aigc-cli upscale init --model %s", err, info.ID)), nil
		}

		img, _, err := decodeImageGuarded(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("cannot decode image: %v", err)), nil
		}

		det, err := up.NewDetector(libPath, modelPath, info.Scale, info.Input(), info.Output())
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("init upscaler: %v", err)), nil
		}
		defer det.Close()

		out, err := det.Upscale(img)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("upscale failed: %v", err)), nil
		}

		if scale := int(req.GetFloat("scale", 0)); scale > 0 && scale != det.Scale() {
			b := img.Bounds()
			dst := image.NewNRGBA(image.Rect(0, 0, b.Dx()*scale, b.Dy()*scale))
			draw.CatmullRom.Scale(dst, dst.Bounds(), out, out.Bounds(), draw.Src, nil)
			out = dst
		}

		output, err := confinedOutputPath(cfg, req.GetString("output_path", ""), path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if output == "" {
			ext := filepath.Ext(path)
			stem := strings.TrimSuffix(filepath.Base(path), ext)
			output = filepath.Join(filepath.Dir(path), stem+"_upscaled.png")
		}

		f, err := os.Create(output)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create output: %v", err)), nil
		}
		err = png.Encode(f, out)
		f.Close()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("save output: %v", err)), nil
		}

		msg := fmt.Sprintf("Upscaled with %s (%s) → %s (%dx%d)",
			info.ID, info.License, output, out.Bounds().Dx(), out.Bounds().Dy())
		return toolResultTextWithMedia(msg, output), nil
	}
}
