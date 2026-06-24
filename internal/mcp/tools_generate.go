package mcp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// parseImageURLs splits a comma-separated string into a string slice.
func parseImageURLs(raw string) []string {
	if raw == "" {
		return nil
	}
	var urls []string
	for _, u := range strings.Split(raw, ",") {
		u = strings.TrimSpace(u)
		if u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

// generateImageHandler creates the handler for generate_image, capturing the config.
func generateImageHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if cfg.APIKey == "" {
			return mcp.NewToolResultError("API Key not configured. Set APIMART_API_KEY env or configure ~/.config/apimart/config.yaml"), nil
		}

		prompt, err := request.RequireString("prompt")
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}

		req := &types.GenerateRequest{
			Model:        request.GetString("model", ""),
			Prompt:       prompt,
			Size:         request.GetString("size", ""),
			Resolution:   request.GetString("resolution", ""),
			Quality:      request.GetString("quality", ""),
			OutputFormat: request.GetString("output_format", ""),
			ImageURLs:    parseImageURLs(request.GetString("image_urls", "")),
			MaskURL:      request.GetString("mask_url", ""),
		}

		// Merge config defaults
		if imgCfg := cfg.Defaults.Image; imgCfg != nil {
			imgCfg.MergeIntoImage(req)
		}

		// Apply defaults for remaining empty fields
		if req.Model == "" {
			return nil, fmt.Errorf("model is required: set model in request or defaults.image.model in config.yaml")
		}
		if req.Size == "" {
			req.Size = "1:1"
		}
		if req.Resolution == "" {
			req.Resolution = "1k"
		}
		if req.Quality == "" {
			req.Quality = "auto"
		}
		if req.OutputFormat == "" {
			req.OutputFormat = "png"
		}

		c := client.New(cfg.APIKey, cfg.BaseURL, cfg.Proxy)

		// Resolve local images if any
		if len(req.ImageURLs) > 0 {
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve image URLs: %v", err)), nil
			}
			req.ImageURLs = resolved
		}
		if req.MaskURL != "" {
			resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve mask URL: %v", err)), nil
			}
			req.MaskURL = resolved[0]
		}

		// Submit
		resp, err := c.Submit(req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Submission failed: %v", err)), nil
		}
		if len(resp.Data) == 0 {
			return mcp.NewToolResultError("Submission returned no tasks"), nil
		}

		taskInfo := resp.Data[0]

		// Poll until complete
		taskData, err := c.PollTask(taskInfo.TaskID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Task polling failed: %v", err)), nil
		}

		// Download images to output directory
		var savedFiles []string
		if taskData.Result != nil && len(taskData.Result.Images) > 0 {
			for i, img := range taskData.Result.Images {
				for j, url := range img.URL {
					ext := filepath.Ext(url)
					if ext == "" {
						ext = ".png"
					}
					filename := filepath.Join(cfg.Output, fmt.Sprintf("apimart_%s_%d_%d%s", taskData.ID, i, j, ext))
					if err := downloadFile(url, filename); err != nil {
						continue
					}
					savedFiles = append(savedFiles, filename)
				}
			}
		}

		// Build result text
		lines := []string{
			fmt.Sprintf("Task ID: %s", taskData.ID),
			fmt.Sprintf("Status: completed"),
			fmt.Sprintf("Time: %ds | Cost: $%.5f (%.4f credits)", taskData.ActualTime, taskData.Cost, taskData.CreditsCost),
		}
		if len(savedFiles) > 0 {
			lines = append(lines, "")
			lines = append(lines, "已保存的图片:")
			for _, f := range savedFiles {
				lines = append(lines, fmt.Sprintf("  %s", f))
			}
		}

		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}
}

// generateVideoHandler creates the handler for generate_video, capturing the config.
// Video generation is async - returns task_id immediately for polling via get_task.
func generateVideoHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if cfg.APIKey == "" {
			return mcp.NewToolResultError("API Key not configured. Set APIMART_API_KEY env or configure ~/.config/apimart/config.yaml"), nil
		}

		prompt, err := request.RequireString("prompt")
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}

		req := &types.VideoGenerateRequest{
			Model:      request.GetString("model", ""),
			Prompt:     prompt,
			Size:       request.GetString("size", ""),
			Resolution: request.GetString("resolution", ""),
			ImageURLs:  parseImageURLs(request.GetString("image_urls", "")),
			VideoURLs:  parseImageURLs(request.GetString("video_urls", "")),
		}

		if d := request.GetInt("duration", 0); d > 0 {
			v := d
			req.Duration = &v
		}
		if request.GetBool("generate_audio", false) {
			v := true
			req.GenerateAudio = &v
		}

		// Merge config defaults
		if videoCfg := cfg.Defaults.Video; videoCfg != nil {
			videoCfg.MergeIntoVideo(req)
		}

		if req.Model == "" {
			return nil, fmt.Errorf("model is required: set model in request or defaults.video.model in config.yaml")
		}
		if req.Size == "" {
			req.Size = "16:9"
		}
		if req.Resolution == "" {
			req.Resolution = "480p"
		}

		c := client.New(cfg.APIKey, cfg.BaseURL, cfg.Proxy)

		// Resolve local images
		if len(req.ImageURLs) > 0 {
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve image URLs: %v", err)), nil
			}
			req.ImageURLs = resolved
		}

		// Submit
		resp, err := c.VideoSubmit(req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Video submission failed: %v", err)), nil
		}
		if len(resp.Data) == 0 {
			return mcp.NewToolResultError("Submission returned no tasks"), nil
		}

		taskInfo := resp.Data[0]

		text := fmt.Sprintf("视频任务已提交。\n\nTask ID: %s\nStatus: %s\n\n视频生成耗时较长（通常 30-180 秒），请使用 get_task 工具传入此 task_id 查询生成结果。", taskInfo.TaskID, taskInfo.Status)
		return mcp.NewToolResultText(text), nil
	}
}

// httpGetBytes performs a simple GET request and returns the body bytes.
func httpGetBytes(url string) ([]byte, error) {
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// downloadFile downloads a URL to a local file path.
func downloadFile(url, dest string) error {
	data, err := httpGetBytes(url)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0644)
}
