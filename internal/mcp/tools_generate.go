package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/cli/image"
	"github.com/martianzhang/aigc-cli/internal/cli/music"
	"github.com/martianzhang/aigc-cli/internal/cli/video"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/gif"
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
// Delegates dispatch to the shared image runner so every supported provider is covered.
func generateImageHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p, providerErr := cfg.resolveProviderRef("image", request.GetString("provider", ""))
		if providerErr != "" {
			return mcp.NewToolResultError(providerErr), nil
		}
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

		saved, err := image.GenerateAndSave(c, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		lines := []string{"已保存的图片:"}
		for _, f := range saved {
			lines = append(lines, fmt.Sprintf("  %s", f))
		}
		return toolResultTextWithMedia(strings.Join(lines, "\n"), saved...), nil
	}
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

		// Coarse-phase progress: the host is told dispatch is starting, then
		// that the long submit → poll phase begins. Silent unless the request
		// carried a progressToken and a session is available.
		reporter := newProgressReporter(ctx, request)
		reporter.report(progressDispatch, progressDispatchMessage(p.ProviderType.String()))
		reporter.report(progressGenerating, progressGeneratingMessage)

		saved, err := video.GenerateAndSave(req)
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

// generateMusicHandler creates the handler for generate_music, capturing the config.
// Delegates dispatch to the shared music runner so every supported provider is covered.
func generateMusicHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p, providerErr := cfg.resolveProviderRef("music", request.GetString("provider", ""))
		if providerErr != "" {
			return mcp.NewToolResultError(providerErr), nil
		}
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

		// Same coarse-phase contract as generate_video: dispatch, then the
		// long submit → poll phase. Silent without progressToken + session.
		reporter := newProgressReporter(ctx, request)
		reporter.report(progressDispatch, progressDispatchMessage(p.ProviderType.String()))
		reporter.report(progressGenerating, progressGeneratingMessage)

		saved, err := music.GenerateAndSave(c, req)
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

// validSpeechFormats is the security whitelist for generate_speech formats;
// the value becomes the saved filename extension.
var validSpeechFormats = map[string]bool{
	"mp3": true, "wav": true, "opus": true, "aac": true, "flac": true, "pcm": true,
}

func validateSpeechFormat(format string) error {
	if !validSpeechFormats[strings.ToLower(format)] {
		return fmt.Errorf("invalid format: %s", format)
	}
	return nil
}

// generateSpeechHandler creates the handler for generate_speech, capturing the config.
func generateSpeechHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p, providerErr := cfg.resolveProviderRef("audio", request.GetString("provider", ""))
		if providerErr != "" {
			return mcp.NewToolResultError(providerErr), nil
		}
		if p.RequiresAPIKey() {
			return mcp.NewToolResultError("API Key not configured"), nil
		}

		input, err := request.RequireString("input")
		if err != nil {
			return mcp.NewToolResultError("input is required"), nil
		}

		format := strings.ToLower(request.GetString("format", "mp3"))
		if err := validateSpeechFormat(format); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		req := &types.AudioSpeechRequest{
			Model:          request.GetString("model", ""),
			Input:          input,
			Voice:          request.GetString("voice", ""),
			ResponseFormat: format,
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

		return toolResultTextWithMedia(fmt.Sprintf("Speech saved: %s\nFormat: %s\nSize: %d bytes\nModel: %s\nVoice: %s",
			filename, req.ResponseFormat, len(audioData), req.Model, req.Voice), filename), nil
	}
}

// transcribeAudioHandler creates the handler for transcribe_audio, capturing the config.
func transcribeAudioHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		p, providerErr := cfg.resolveProviderRef("audio", request.GetString("provider", ""))
		if providerErr != "" {
			return mcp.NewToolResultError(providerErr), nil
		}
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
