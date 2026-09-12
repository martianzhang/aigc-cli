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
	"github.com/martianzhang/aigc-cli/internal/gif"
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
		p := cfg.cmdProvider("image")
		if p.RequiresAPIKey() {
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
			Ratio:        request.GetString("ratio", ""),
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
		if err := req.ValidateBackground(); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		c := client.NewFromProvider(p)

		switch p.ProviderType {
		case provider.OpenRouter:
			return handleMCPOpenRouterImage(c, req, cfg.Output)
		case provider.Agnes:
			return handleMCPAgnesImage(c, req, cfg.Output)
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

// handleMCPAgnesImage generates an image via Agnes API (sync).
// Transforms ImageURLs into extra_body.image and handles ratio for 2.1 tiered sizing.
func handleMCPAgnesImage(c client.APIClient, req *types.GenerateRequest, outputDir string) (*mcp.CallToolResult, error) {
	// Transform ImageURLs into extra_body.image (Agnes requires it nested).
	if len(req.ImageURLs) > 0 {
		if req.ExtraBody == nil {
			req.ExtraBody = make(map[string]interface{})
		}
		req.ExtraBody["image"] = req.ImageURLs
		req.ImageURLs = nil
	}
	if req.Ratio != "" {
		if req.ExtraBody == nil {
			req.ExtraBody = make(map[string]interface{})
		}
		req.ExtraBody["ratio"] = req.Ratio
	}
	if req.ResponseFormat != "" {
		if req.ExtraBody == nil {
			req.ExtraBody = make(map[string]interface{})
		}
		req.ExtraBody["response_format"] = req.ResponseFormat
		req.ResponseFormat = ""
	}

	resp, err := c.ImageGenerateSync(req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Agnes image generation failed: %v", err)), nil
	}

	var savedFiles []string
	for i, img := range resp.Data {
		if img.B64JSON != "" {
			raw, decErr := base64.StdEncoding.DecodeString(img.B64JSON)
			if decErr != nil {
				continue
			}
			ts := time.Now().Unix()
			filename := filepath.Join(outputDir, fmt.Sprintf("agnes_%d_%d.png", ts, i))
			if err := os.WriteFile(filename, raw, 0644); err != nil {
				continue
			}
			savedFiles = append(savedFiles, filename)
		} else if img.URL != "" {
			filename, dlErr := service.DownloadFile(img.URL, outputDir, fmt.Sprintf("agnes_%d_%d", time.Now().Unix(), i))
			if dlErr != nil {
				continue
			}
			savedFiles = append(savedFiles, filename)
		}
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
// Video generation is async—submits, polls, downloads, and optionally converts to GIF.
func generateVideoHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p := cfg.cmdProvider("video")
		if p.RequiresAPIKey() {
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

		gifOpts := gifRequestOptions{
			Enabled: request.GetBool("gif", false),
			Width:   request.GetInt("gif_width", 160),
		}
		if raw := request.GetString("crop_margin", ""); raw != "" {
			m, perr := gif.ParseCropMargin(raw)
			if perr != nil {
				return mcp.NewToolResultError(perr.Error()), nil
			}
			gifOpts.CropMargin = m
		}

		c := client.NewFromProvider(p)

		var saved []string
		switch p.ProviderType {
		case provider.OpenRouter:
			saved, err = mcpOpenRouterVideo(c, req, cfg.Output)
		case provider.Agnes:
			saved, err = mcpAgnesVideo(c, req, cfg.Output)
		default:
			saved, err = mcpAPIMartVideo(c, req, cfg.Output)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		lines := []string{"视频生成完成。"}
		lines = append(lines, "")
		lines = append(lines, "已保存的视频:")
		for _, f := range saved {
			lines = append(lines, fmt.Sprintf("  %s", f))
		}

		if gifOpts.Enabled {
			gifs, gerr := convertVideosToGIF(saved, gifOpts)
			if gerr != nil {
				lines = append(lines, "", fmt.Sprintf("⚠️ GIF 转换失败: %v", gerr))
			} else {
				lines = append(lines, "", "已保存的 GIF:")
				for _, f := range gifs {
					lines = append(lines, fmt.Sprintf("  %s", f))
				}
			}
		}

		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}
}

// gifRequestOptions holds --gif equivalent options for MCP generate_video.
type gifRequestOptions struct {
	Enabled    bool
	Width      int
	CropMargin gif.CropMargins
}

// convertVideosToGIF converts each saved .mp4 to a GIF using the internal gif package.
// Returns the list of GIF paths; non-mp4 files are skipped.
func convertVideosToGIF(saved []string, opts gifRequestOptions) ([]string, error) {
	if !gif.Available() {
		return nil, gif.MissingHint()
	}
	var gifs []string
	for _, f := range saved {
		if !strings.HasSuffix(strings.ToLower(f), ".mp4") {
			continue
		}
		p, err := gif.Convert(gif.ConvertOptions{
			Input:      f,
			Width:      opts.Width,
			CropMargin: opts.CropMargin,
		})
		if err != nil {
			return gifs, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		gifs = append(gifs, p)
	}
	return gifs, nil
}

// mcpOpenRouterVideo submits an OpenRouter video job, polls to completion, and downloads the result.
func mcpOpenRouterVideo(c client.APIClient, req *types.VideoGenerateRequest, outputDir string) ([]string, error) {
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
		return nil, fmt.Errorf("OpenRouter video submission failed: %w", err)
	}
	pollResp, err := c.OpenRouterVideoPollUntilComplete(submitResp.PollingURL, 30*time.Second, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("video polling failed: %w", err)
	}
	if len(pollResp.UnsignedURLs) == 0 {
		return nil, fmt.Errorf("video job completed but no download URLs returned")
	}

	var saved []string
	for i, u := range pollResp.UnsignedURLs {
		filename, dlErr := service.DownloadFile(u, outputDir, fmt.Sprintf("video_%s_%d", submitResp.ID, i))
		if dlErr != nil {
			return saved, fmt.Errorf("failed to download video %d: %w", i, dlErr)
		}
		saved = append(saved, filename)
	}
	return saved, nil
}

// mcpAgnesVideo submits an agnes.ai video task, polls to completion, and downloads the result.
func mcpAgnesVideo(c client.APIClient, req *types.VideoGenerateRequest, outputDir string) ([]string, error) {
	cc, ok := c.(*client.Client)
	if !ok {
		return nil, fmt.Errorf("unsupported client type for agnes video")
	}
	createResp, err := cc.AgnesVideoSubmit(req)
	if err != nil {
		return nil, fmt.Errorf("agnes video submission failed: %w", err)
	}
	videoID := createResp.VideoID
	if videoID == "" {
		videoID = createResp.TaskID
	}
	if videoID == "" {
		return nil, fmt.Errorf("agnes video submission returned no video id")
	}

	const (
		pollInterval = 15 * time.Second
		maxWait      = 10 * time.Minute
	)
	start := time.Now()
	var videoURL string
	for {
		if time.Since(start) > maxWait {
			return nil, fmt.Errorf("agnes video polling timed out after %v", maxWait)
		}
		queryResp, qerr := cc.AgnesVideoQuery(videoID, req.Model)
		if qerr != nil {
			return nil, fmt.Errorf("polling failed: %w", qerr)
		}
		switch queryResp.Status {
		case "completed", "succeeded", "success":
			videoURL = queryResp.URL
			if videoURL == "" && queryResp.Metadata != nil {
				videoURL = queryResp.Metadata.URL
			}
			if videoURL == "" {
				return nil, fmt.Errorf("agnes video completed but no url returned")
			}
		case "failed", "failure":
			return nil, fmt.Errorf("agnes video generation failed: status=%s", queryResp.Status)
		case "cancelled", "expired":
			return nil, fmt.Errorf("agnes video generation %s", queryResp.Status)
		default:
			time.Sleep(pollInterval)
		}
		if videoURL != "" {
			break
		}
	}

	filename, err := service.DownloadFile(videoURL, outputDir, fmt.Sprintf("video_agnes_%s", videoID))
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}
	return []string{filename}, nil
}

// mcpAPIMartVideo submits an APIMart video task, polls to completion, and downloads the result.
func mcpAPIMartVideo(c client.APIClient, req *types.VideoGenerateRequest, outputDir string) ([]string, error) {
	// Resolve local images
	if len(req.ImageURLs) > 0 {
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve image URLs: %w", err)
		}
		req.ImageURLs = resolved
	}

	resp, err := c.VideoSubmit(req)
	if err != nil {
		return nil, fmt.Errorf("video submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("submission returned no tasks")
	}

	taskInfo := resp.Data[0]
	taskData, err := c.PollTask(taskInfo.TaskID)
	if err != nil {
		return nil, fmt.Errorf("task polling failed: %w", err)
	}

	var saved []string
	if taskData.Result != nil && len(taskData.Result.Videos) > 0 {
		for i, vid := range taskData.Result.Videos {
			for j, url := range vid.URL {
				filename, dlErr := service.DownloadFile(url, outputDir, fmt.Sprintf("video_%s_%d_%d", taskData.ID, i, j))
				if dlErr != nil {
					return saved, fmt.Errorf("failed to download video %d-%d: %w", i, j, dlErr)
				}
				saved = append(saved, filename)
			}
		}
	}
	if len(saved) == 0 {
		return nil, fmt.Errorf("task completed but no videos found")
	}
	return saved, nil
}

// generateMusicHandler creates the handler for generate_music, capturing the config.
// Supports APIMart (suno / flowmusic async task) and OpenRouter (Google Lyria sync streaming).
func generateMusicHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p := cfg.cmdProvider("music")
		if p.RequiresAPIKey() {
			return mcp.NewToolResultError("API Key not configured"), nil
		}

		prompt, err := request.RequireString("prompt")
		if err != nil {
			return mcp.NewToolResultError("prompt is required"), nil
		}

		req := &types.MusicGenerateRequest{
			Model:  request.GetString("model", ""),
			Prompt: prompt,
		}
		if d := request.GetInt("duration", 0); d > 0 {
			v := d
			req.Duration = &v
		}
		if request.GetBool("instrumental", false) {
			v := true
			req.Instrumental = &v
		}

		// Merge config defaults
		if musicCfg := cfg.Defaults.Music; musicCfg != nil {
			musicCfg.MergeIntoMusic(req)
		}

		c := client.NewFromProvider(p)

		var saved []string
		switch p.ProviderType {
		case provider.OpenRouter:
			saved, err = mcpOpenRouterMusic(c, req, cfg.Output)
		default:
			saved, err = mcpAPIMartMusic(c, req, cfg.Output)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		lines := []string{"音乐生成完成。"}
		lines = append(lines, "")
		lines = append(lines, "已保存的音乐:")
		for _, f := range saved {
			lines = append(lines, fmt.Sprintf("  %s", f))
		}
		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}
}

// mcpOpenRouterMusic streams a music generation via OpenRouter's chat
// completions endpoint and saves the decoded audio to outputDir.
func mcpOpenRouterMusic(c client.APIClient, req *types.MusicGenerateRequest, outputDir string) ([]string, error) {
	model := req.Model
	if model == "" {
		model = "google/lyria-3-clip-preview"
	}

	text := req.Prompt
	if req.Instrumental != nil && *req.Instrumental {
		text = "[Instrumental] " + text
	}
	if req.Duration != nil {
		text += fmt.Sprintf("\n\nTarget duration: about %d seconds.", *req.Duration)
	}

	format := req.Format
	if format == "" {
		format = "mp3"
	}

	orReq := &types.OpenRouterMusicRequest{
		Model:      model,
		Messages:   []types.OpenRouterMusicMessage{{Role: "user", Content: text}},
		Modalities: []string{"text", "audio"},
		Audio:      &types.OpenRouterAudioConfig{Format: format},
		Stream:     true,
	}

	audio, _, err := c.OpenRouterMusicGenerate(orReq)
	if err != nil {
		return nil, fmt.Errorf("OpenRouter music generation failed: %w", err)
	}

	ts := time.Now().Unix()
	filename := filepath.Join(outputDir, fmt.Sprintf("music_%d.%s", ts, format))
	if err := os.WriteFile(filename, audio, 0644); err != nil {
		return nil, fmt.Errorf("failed to save music: %w", err)
	}
	return []string{filename}, nil
}

// mcpAPIMartMusic submits an APIMart music task, polls to completion, and downloads the tracks.
func mcpAPIMartMusic(c client.APIClient, req *types.MusicGenerateRequest, outputDir string) ([]string, error) {
	body := buildMCPMusicBody(req)

	resp, err := c.MusicSubmit(body)
	if err != nil {
		return nil, fmt.Errorf("music submission failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("submission returned no tasks")
	}

	taskData, err := c.MusicPollTask(resp.Data[0].TaskID)
	if err != nil {
		return nil, fmt.Errorf("task polling failed: %w", err)
	}
	if taskData.Result == nil || len(taskData.Result.Music) == 0 {
		return nil, fmt.Errorf("task completed but no music found")
	}

	var saved []string
	for i, track := range taskData.Result.Music {
		url := mcpMusicTrackURL(track)
		if url == "" {
			continue
		}
		filename, dlErr := service.DownloadFile(url, outputDir, fmt.Sprintf("music_%s_%d", taskData.ID, i))
		if dlErr != nil {
			return saved, fmt.Errorf("failed to download music %d: %w", i, dlErr)
		}
		saved = append(saved, filename)
	}
	if len(saved) == 0 {
		return nil, fmt.Errorf("task completed but no downloadable music found")
	}
	return saved, nil
}

// buildMCPMusicBody maps a typed request to the backend-native music body.
// The MCP tool exposes only prompt/model/duration/instrumental, so the suno and
// flowmusic shapes are built from those plus format.
func buildMCPMusicBody(req *types.MusicGenerateRequest) map[string]any {
	model := req.Model
	if model == "" {
		model = "suno"
	}
	backend := "suno"
	if strings.Contains(strings.ToLower(model), "flowmusic") {
		backend = "flowmusic"
	}

	body := map[string]any{"model": model}
	if backend == "flowmusic" {
		body["sound_prompt"] = req.Prompt
		length := 120
		if req.Duration != nil {
			length = *req.Duration
		}
		body["length"] = length
		return body
	}

	instrumental := false
	if req.Instrumental != nil {
		instrumental = *req.Instrumental
	}
	body["instrumental"] = instrumental
	body["custom"] = false
	if req.Prompt != "" {
		body["prompt"] = req.Prompt
	}
	body["version"] = "v6"
	if req.Duration != nil {
		body["duration"] = *req.Duration
	}
	if req.Format != "" {
		body["audio_format"] = req.Format
	}
	return body
}

// mcpMusicTrackURL picks the preferred downloadable URL from a track.
func mcpMusicTrackURL(track types.MusicTrack) string {
	switch {
	case track.AudioURL != "":
		return track.AudioURL
	case track.WAVURL != "":
		return track.WAVURL
	case track.FileURL != "":
		return track.FileURL
	case track.VideoURL != "":
		return track.VideoURL
	default:
		return ""
	}
}

// generateSpeechHandler creates the handler for generate_speech, capturing the config.
func generateSpeechHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p := cfg.cmdProvider("audio")
		if p.RequiresAPIKey() {
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

		c := client.NewFromProvider(p)
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
		p := cfg.cmdProvider("audio")
		if p.RequiresAPIKey() {
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

		c := client.NewFromProvider(p)
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
