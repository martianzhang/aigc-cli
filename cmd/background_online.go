package cmd

import (
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// generateOnlineBackground generates a background-modified image via the image API.
// Reuses the existing image generation pipeline (sync for OpenAI, async for APIMart, native for Ollama).
func generateOnlineBackground(imagePath string, p *provider.EffectiveProvider, defaultPrompt, userPrompt string) (string, error) {
	prompt := userPrompt
	if prompt == "" {
		prompt = defaultPrompt
	}
	req := &types.GenerateRequest{
		Model:     p.Model,
		Prompt:    prompt,
		ImageURLs: []string{imagePath},
	}
	// Ollama native API
	if p.Type == types.ProviderOllama || provider.IsLocalEndpoint(p.BaseURL) {
		saved, err := ollamaGenerateImages(p.BaseURL, req)
		if err != nil {
			return "", err
		}
		if len(saved) == 0 {
			return "", fmt.Errorf("no images saved")
		}
		return saved[0], nil
	}
	c := client.NewFromProvider(p)
	if len(req.ImageURLs) > 0 {
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return "", fmt.Errorf("resolve image failed: %w", err)
		}
		req.ImageURLs = resolved
	}
	// APIMart: async submit → poll → download (reusing existing helpers)
	if p.ProviderType == provider.APIMart {
		subResp, err := c.Submit(req)
		if err != nil {
			return "", fmt.Errorf("submit failed: %w", err)
		}
		if len(subResp.Data) == 0 {
			return "", fmt.Errorf("submit returned no tasks")
		}
		taskData, err := c.PollTask(subResp.Data[0].TaskID)
		if err != nil {
			return "", fmt.Errorf("poll failed: %w", err)
		}
		if taskData.Result == nil || len(taskData.Result.Images) == 0 {
			return "", fmt.Errorf("no images in task result")
		}
		saved, err := downloadImages(taskData.Result.Images, taskData.ID)
		if err != nil {
			return "", fmt.Errorf("download failed: %w", err)
		}
		if len(saved) == 0 {
			return "", fmt.Errorf("no images downloaded")
		}
		return saved[0], nil
	}
	// Sync provider (OpenAI, OpenRouter, etc.)
	resp, err := c.ImageGenerateSync(req)
	if err != nil {
		return "", fmt.Errorf("online generation failed: %w", err)
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("no images returned")
	}
	img := resp.Data[0]
	prefix := fmt.Sprintf("bg_online_%d", time.Now().Unix())
	if img.B64JSON != "" {
		return service.SaveBase64Image(shared.OutputDir, prefix, img.B64JSON, 0)
	}
	if img.URL != "" {
		return service.DownloadFile(img.URL, shared.OutputDir, prefix)
	}
	return "", fmt.Errorf("no image data in response")
}
