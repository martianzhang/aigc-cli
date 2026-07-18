package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
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
// Supports APIMart (async task) and OpenRouter (dedicated image API).
func generateImageHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if cfg.APIKey == "" {
			return mcp.NewToolResultError("API Key not configured"), nil
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
			Background:   request.GetString("background", ""),
		}

		// Merge config defaults
		if imgCfg := cfg.Defaults.Image; imgCfg != nil {
			imgCfg.MergeIntoImage(req)
		}

		// Apply defaults
		if req.Model == "" {
			return mcp.NewToolResultError("model is required: set model in request or defaults.image.model in config.yaml"), nil
		}
		if req.Size == "" {
			req.Size = "1:1"
		}
		if req.Quality == "" {
			req.Quality = "auto"
		}
		if req.OutputFormat == "" {
			req.OutputFormat = "png"
		}

		c := client.New(cfg.APIKey, cfg.BaseURL, cfg.Proxy)
		p := provider.Detect(cfg.BaseURL)

		switch p {
		case provider.OpenRouter:
			return handleMCPOpenRouterImage(c, req, cfg.Output)
		default:
			return handleMCPAPIMartImage(c, req, cfg.Output)
		}
	}
}

// handleMCPOpenRouterImage generates an image via OpenRouter's dedicated image API.
func handleMCPOpenRouterImage(c client.APIClient, req *types.GenerateRequest, outputDir string) (*mcp.CallToolResult, error) {
	resp, err := c.OpenRouterDedicatedImage(req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("OpenRouter image generation failed: %v", err)), nil
	}

	var savedFiles []string
	for i, img := range resp.Data {
		if img.B64JSON == "" {
			continue
		}
		raw, decErr := base64.StdEncoding.DecodeString(img.B64JSON)
		if decErr != nil {
			continue
		}
		ts := time.Now().Unix()
		ext := ".png"
		filename := filepath.Join(outputDir, fmt.Sprintf("image_%d_%d%s", ts, i, ext))
		if err := os.WriteFile(filename, raw, 0644); err != nil {
			continue
		}
		savedFiles = append(savedFiles, filename)
	}

	lines := []string{fmt.Sprintf("Created: %d", resp.Created)}
	if len(savedFiles) > 0 {
		lines = append(lines, "")
		lines = append(lines, "已保存的图片:")
		for _, f := range savedFiles {
			lines = append(lines, fmt.Sprintf("  %s", f))
		}
	}
	if resp.Usage != nil && resp.Usage.Cost > 0 {
		lines = append(lines, fmt.Sprintf("Cost: $%.5f", resp.Usage.Cost))
	}
	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

// handleMCPAPIMartImage generates an image via APIMart async task API.
func handleMCPAPIMartImage(c client.APIClient, req *types.GenerateRequest, outputDir string) (*mcp.CallToolResult, error) {
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

	resp, err := c.Submit(req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Submission failed: %v", err)), nil
	}
	if len(resp.Data) == 0 {
		return mcp.NewToolResultError("Submission returned no tasks"), nil
	}

	taskInfo := resp.Data[0]
	taskData, err := c.PollTask(taskInfo.TaskID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Task polling failed: %v", err)), nil
	}

	var savedFiles []string
	if taskData.Result != nil && len(taskData.Result.Images) > 0 {
		for i, img := range taskData.Result.Images {
			for j, url := range img.URL {
				filename, err := service.DownloadFile(url, outputDir, fmt.Sprintf("image_%s_%d_%d", taskData.ID, i, j))
				if err != nil {
					continue
				}
				savedFiles = append(savedFiles, filename)
			}
		}
	}

	lines := []string{
		fmt.Sprintf("Task ID: %s", taskData.ID),
		"Status: completed",
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

// generateVideoHandler creates the handler for generate_video, capturing the config.
// Video generation is async—returns a task/job ID for polling.
func generateVideoHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if cfg.APIKey == "" {
			return mcp.NewToolResultError("API Key not configured"), nil
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
			return mcp.NewToolResultError("model is required: set model in request or defaults.video.model in config.yaml"), nil
		}
		if req.Size == "" {
			req.Size = "16:9"
		}
		if req.Resolution == "" {
			req.Resolution = "480p"
		}

		c := client.New(cfg.APIKey, cfg.BaseURL, cfg.Proxy)
		p := provider.Detect(cfg.BaseURL)

		switch p {
		case provider.OpenRouter:
			return handleMCPOpenRouterVideo(c, req)
		default:
			return handleMCPAPIMartVideo(c, req)
		}
	}
}

// handleMCPOpenRouterVideo submits a video job via OpenRouter and saves the job info.
func handleMCPOpenRouterVideo(c client.APIClient, req *types.VideoGenerateRequest) (*mcp.CallToolResult, error) {
	orReq := &types.OpenRouterVideoRequest{
		Model:         req.Model,
		Prompt:        req.Prompt,
		AspectRatio:   req.Size,
		Resolution:    req.Resolution,
		Duration:      req.Duration,
		Seed:          req.Seed,
		GenerateAudio: req.GenerateAudio,
	}
	for _, u := range req.ImageURLs {
		orReq.FrameImages = append(orReq.FrameImages, types.OpenRouterFrameImage{
			Type: "image_url", FrameType: "first_frame",
			ImageURL: struct {
				URL string `json:"url"`
			}{URL: u},
		})
	}

	submitResp, err := c.OpenRouterVideoSubmit(orReq)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("OpenRouter video submission failed: %v", err)), nil
	}

	text := fmt.Sprintf("视频任务已提交。\n\nJob ID: %s\nStatus: %s\n\n视频生成耗时较长（30秒-几分钟），稍后可使用 get_task 工具传入 Job ID 查询结果。\npolling_url: %s",
		submitResp.ID, submitResp.Status, submitResp.PollingURL)
	return mcp.NewToolResultText(text), nil
}

// handleMCPAPIMartVideo submits a video job via APIMart async task API.
func handleMCPAPIMartVideo(c client.APIClient, req *types.VideoGenerateRequest) (*mcp.CallToolResult, error) {
	// Resolve local images
	if len(req.ImageURLs) > 0 {
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resolve image URLs: %v", err)), nil
		}
		req.ImageURLs = resolved
	}

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

// generateSpeechHandler creates the handler for generate_speech, capturing the config.
func generateSpeechHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if cfg.APIKey == "" {
			return mcp.NewToolResultError("API Key not configured"), nil
		}

		input, err := request.RequireString("input")
		if err != nil {
			return mcp.NewToolResultError("input is required"), nil
		}

		req := &types.AudioSpeechRequest{
			Model:          request.GetString("model", ""),
			Input:          input,
			Voice:          request.GetString("voice", ""),
			ResponseFormat: request.GetString("format", "mp3"),
		}

		if req.Model == "" {
			if cfg.Defaults.Audio != nil && cfg.Defaults.Audio.SpeakModel != "" {
				req.Model = cfg.Defaults.Audio.SpeakModel
			} else {
				req.Model = "gpt-4o-mini-tts"
			}
		}
		if req.Voice == "" {
			if cfg.Defaults.Audio != nil && cfg.Defaults.Audio.Voice != "" {
				req.Voice = cfg.Defaults.Audio.Voice
			}
		}
		if req.Voice == "" {
			return mcp.NewToolResultError("voice is required"), nil
		}

		c := client.New(cfg.APIKey, cfg.BaseURL, cfg.Proxy)
		audioData, _, err := c.AudioSpeech(req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("TTS failed: %v", err)), nil
		}

		ext := "." + req.ResponseFormat
		ts := time.Now().Unix()
		filename := filepath.Join(cfg.Output, fmt.Sprintf("speech_%d%s", ts, ext))
		if err := os.WriteFile(filename, audioData, 0644); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to save audio: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Speech saved: %s\nFormat: %s\nSize: %d bytes\nModel: %s\nVoice: %s",
			filename, req.ResponseFormat, len(audioData), req.Model, req.Voice)), nil
	}
}

// transcribeAudioHandler creates the handler for transcribe_audio, capturing the config.
func transcribeAudioHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if cfg.APIKey == "" {
			return mcp.NewToolResultError("API Key not configured"), nil
		}

		filePath, err := request.RequireString("file_path")
		if err != nil {
			return mcp.NewToolResultError("file_path is required"), nil
		}

		model := request.GetString("model", "")
		if model == "" {
			if cfg.Defaults.Audio != nil && cfg.Defaults.Audio.TranscribeModel != "" {
				model = cfg.Defaults.Audio.TranscribeModel
			} else {
				model = "whisper-1"
			}
		}

		language := request.GetString("language", "")

		c := client.New(cfg.APIKey, cfg.BaseURL, cfg.Proxy)
		resp, err := c.AudioTranscribeMultipart(model, filePath, language)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("STT failed: %v", err)), nil
		}

		text := resp.Text
		detail := fmt.Sprintf("Model: %s\n", model)
		if resp.Usage != nil {
			detail += fmt.Sprintf("Audio: %.1fs | Cost: $%.5f\n", resp.Usage.Seconds, resp.Usage.Cost)
		}
		detail += "\n" + text

		return mcp.NewToolResultText(detail), nil
	}
}
