package mcp

import (
	"context"
	"fmt"
	"image"
	imagejpeg "image/jpeg"
	imagepng "image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/watermark"
)

// isLocalImageFile returns true for recognized image file extensions.
func isLocalImageFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" ||
		ext == ".gif" || ext == ".bmp" || ext == ".svg" ||
		ext == ".avif" || ext == ".heic" || ext == ".jxl"
}

// resolveAbsPath resolves a possibly-relative path to an absolute one.
func resolveAbsPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// defaultCleanPath mirrors cmd/detect.go's cleanPath for the no-output case.
func defaultCleanPath(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "_clean" + ext
}

// removeWatermarkHandler handles the remove_watermark tool call.
func removeWatermarkHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}
		if !isLocalImageFile(path) {
			return mcp.NewToolResultText("Only image files (.jpg/.jpeg/.png/.webp/.gif/.bmp/.avif/.heic/.jxl) are supported."), nil
		}
		path = resolveAbsPath(path)

		producer := req.GetString("producer", "")
		outputPath := req.GetString("output_path", "")

		loadCustomWatermarks()

		res, err := watermark.RemoveFileHinted(path, outputPath, producer)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("remove failed: %v", err)), nil
		}
		if !res.Removed {
			return mcp.NewToolResultText("No visible AI watermark detected/removed."), nil
		}

		out := outputPath
		if out == "" {
			out = defaultCleanPath(path)
		}
		return mcp.NewToolResultText(fmt.Sprintf("Watermark removed (engine: %s). Output: %s\n\n⚠️ 合规提醒: 请确保您有权处理该图片。", res.Name, out)), nil
	}
}

// loadCustomWatermarks loads all .watermark.png files from the config directory.
func loadCustomWatermarks() {
	home, _ := os.UserHomeDir()
	if home == "" {
		return
	}
	dir := filepath.Join(home, ".config", "aigc-cli", "watermark")
	watermark.LoadWatermarkPNGsFromDir(dir)
}

// addWatermarkHandler handles the add_watermark tool call.
func addWatermarkHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}
		if !isLocalImageFile(path) {
			return mcp.NewToolResultText("Only image files (.jpg/.jpeg/.png/.webp/.gif/.bmp/.avif/.heic/.jxl) are supported."), nil
		}
		path = resolveAbsPath(path)

		producer, err := req.RequireString("producer")
		if err != nil {
			return mcp.NewToolResultError("producer is required (known: gemini, or custom text)"), nil
		}

		outputPath := req.GetString("output_path", "")

		res, err := watermark.AddWatermarkFile(path, outputPath, producer)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("add failed: %v", err)), nil
		}

		out := outputPath
		if out == "" {
			ext := filepath.Ext(path)
			out = strings.TrimSuffix(path, ext) + "_watermarked.png"
		}
		return mcp.NewToolResultText(fmt.Sprintf("Watermark added (engine: %s). Output: %s", res.Name, out)), nil
	}
}

func newCropWatermarkTool() mcp.Tool {
	return mcp.NewTool("crop_watermark",
		mcp.WithDescription("裁切图片以去除水印。通用方法，无需学习水印模板。自动检测水印位置并裁切，适合边角水印。"),
		mcp.WithString("file_path",
			mcp.Required(),
			mcp.Description("图片文件的本地路径"),
		),
		mcp.WithString("target",
			mcp.Description("裁切目标：\"auto\"（自动检测）、\"n%\"（保留比例，如 \"97%\"）、\"WxH\"（目标尺寸，如 \"1920x1080\"）。默认 auto"),
		),
		mcp.WithString("output_path",
			mcp.Description("输出文件路径（可选，默认覆盖原文件或加 _clean 后缀）"),
		),
	)
}

func cropWatermarkHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}
		if !isLocalImageFile(path) {
			return mcp.NewToolResultText("Only image files (.jpg/.jpeg/.png/.webp/.gif/.bmp/.avif/.heic/.jxl) are supported."), nil
		}
		path = resolveAbsPath(path)

		target := req.GetString("target", "auto")
		outputPath := req.GetString("output_path", "")

		f, err := os.Open(path)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("open failed: %v", err)), nil
		}
		defer f.Close()

		img, _, err := image.Decode(f)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("decode failed: %v", err)), nil
		}

		b := img.Bounds()
		imgW, imgH := b.Dx(), b.Dy()

		var bounds watermark.CropBounds

		if target == "" || target == "auto" {
			regions := watermark.DetectWatermarkRegions(img)
			if len(regions) > 0 {
				bounds = watermark.ComputeCropBounds(imgW, imgH, regions)
				if !bounds.Valid {
					// Fall back to default 90% crop when auto detection is ambiguous
					marginRatio := 0.05
					marginX := int(float64(imgW) * marginRatio)
					marginY := int(float64(imgH) * marginRatio)
					bounds = watermark.CropBounds{
						X: marginX, Y: marginY,
						W: imgW - marginX*2, H: imgH - marginY*2,
						Valid: true,
					}
				}
			} else {
				marginRatio := 0.05
				marginX := int(float64(imgW) * marginRatio)
				marginY := int(float64(imgH) * marginRatio)
				bounds = watermark.CropBounds{
					X: marginX, Y: marginY,
					W: imgW - marginX*2, H: imgH - marginY*2,
					Valid: true,
				}
			}
		} else if strings.HasSuffix(target, "%") {
			pctStr := strings.TrimSuffix(target, "%")
			pct, parseErr := strconv.ParseFloat(pctStr, 64)
			if parseErr != nil || pct <= 0 || pct > 100 {
				return mcp.NewToolResultError(fmt.Sprintf("invalid percentage: %s", target)), nil
			}
			keepRatio := pct / 100.0
			newW := int(float64(imgW) * keepRatio)
			newH := int(float64(imgH) * keepRatio)
			x := (imgW - newW) / 2
			y := (imgH - newH) / 2
			bounds = watermark.CropBounds{X: x, Y: y, W: newW, H: newH, Valid: true}
		} else {
			parts := strings.Split(strings.ToLower(target), "x")
			if len(parts) != 2 {
				return mcp.NewToolResultError(fmt.Sprintf("invalid target format: %s (expected \"auto\", \"n%%\", or \"WxH\")", target)), nil
			}
			cropW, wErr := strconv.Atoi(strings.TrimSpace(parts[0]))
			cropH, hErr := strconv.Atoi(strings.TrimSpace(parts[1]))
			if wErr != nil || hErr != nil || cropW <= 0 || cropH <= 0 {
				return mcp.NewToolResultError(fmt.Sprintf("invalid target dimensions: %s", target)), nil
			}
			x := (imgW - cropW) / 2
			y := (imgH - cropH) / 2
			bounds = watermark.CropBounds{X: x, Y: y, W: cropW, H: cropH, Valid: true}
		}

		if bounds.W < 100 || bounds.H < 100 {
			return mcp.NewToolResultText("Crop area too small (minimum 100x100)."), nil
		}

		cropped := image.NewRGBA(image.Rect(0, 0, bounds.W, bounds.H))
		for y := 0; y < bounds.H; y++ {
			for x := 0; x < bounds.W; x++ {
				cropped.Set(x, y, img.At(bounds.X+x, bounds.Y+y))
			}
		}

		if outputPath == "" {
			outputPath = defaultCleanPath(path)
		}

		out, err := os.Create(outputPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("create output failed: %v", err)), nil
		}
		defer out.Close()

		ext := strings.ToLower(filepath.Ext(outputPath))
		switch ext {
		case ".jpg", ".jpeg", ".jfif":
			q := watermark.EstimateJPEGQuality(path)
			if err := imagejpeg.Encode(out, cropped, &imagejpeg.Options{Quality: q}); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("encode failed: %v", err)), nil
			}
		default:
			if err := imagepng.Encode(out, cropped); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("encode failed: %v", err)), nil
			}
		}

		return mcp.NewToolResultText(fmt.Sprintf("Cropped: %dx%d -> %dx%d. Output: %s", imgW, imgH, bounds.W, bounds.H, outputPath)), nil
	}
}
