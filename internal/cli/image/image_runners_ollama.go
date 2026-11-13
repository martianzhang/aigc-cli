package image

import (
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/clientfactory"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// ollamaGenerateImages sends a request to Ollama's /api/generate and returns saved filenames.
func ollamaGenerateImages(baseURL string, req *types.GenerateRequest) ([]string, error) {
	return service.OllamaGenerateImages(baseURL, req, options.Shared.OutputDir,
		clientfactory.HTTPTimeout("image", client.ImageTimeout))
}

// runOllamaImage handles image generation via Ollama's native /api/generate endpoint.
func runOllamaImage(c client.APIClient, req *types.GenerateRequest, ctx *imageDispatchCtx) ([]string, error) {
	start := time.Now()
	saved, err := ollamaGenerateImages(c.BaseURL(), req)
	if err != nil {
		return nil, err
	}
	ui.Printf("Model: %s\n", req.Model)
	ui.Printf("Duration: %.1fs\n", time.Since(start).Seconds())
	service.SavePromptBestEffort(options.Shared.SavePrompt, options.Shared.OutputDir, "image", promptTaskID("image_ollama", 0), req.Prompt)
	for i, f := range saved {
		ui.Printf("Image %d: %s\n", i+1, f)
	}
	postProcessImages(saved)
	printImageSummary(ctx, req, saved, "", 0)
	return saved, nil
}
