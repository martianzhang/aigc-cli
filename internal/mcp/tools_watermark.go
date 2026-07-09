package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/apimart-cli/internal/watermark"
)

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
// Detects and removes a visible AI watermark (doubao/jimeng/baidu/zhipu/...).
func removeWatermarkHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}
		path = resolveAbsPath(path)

		producer := req.GetString("producer", "")
		outputPath := req.GetString("output_path", "")

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
		return mcp.NewToolResultText(fmt.Sprintf("Watermark removed (engine: %s). Output: %s\n\n⚠️ 合规提醒: 请确保您有权处理该图片。本功能仅限合法用途，禁止去除他人版权水印。", res.Name, out)), nil
	}
}

// addWatermarkHandler handles the add_watermark tool call.
// Adds a visible AI watermark for testing purposes.
func addWatermarkHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}
		path = resolveAbsPath(path)

		producer, err := req.RequireString("producer")
		if err != nil {
			return mcp.NewToolResultError("producer is required (known: gemini/doubao/jimeng/baidu/zhipu, or custom text)"), nil
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
