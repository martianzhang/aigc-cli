package image

import (
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ollamaGenerateImages sends a request to Ollama's /api/generate and returns saved filenames.
func ollamaGenerateImages(baseURL string, req *types.GenerateRequest) ([]string, error) {
	return service.OllamaGenerateImages(baseURL, req, options.Shared.OutputDir)
}

// runOllamaImage handles image generation via Ollama's native /api/generate endpoint.
func runOllamaImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()
	saved, err := ollamaGenerateImages(c.BaseURL(), req)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Duration: %.1fs\n", time.Since(start).Seconds())
	for i, f := range saved {
		fmt.Printf("Image %d: %s\n", i+1, f)
	}
	postProcessImages(saved)
	return saved, nil
}
