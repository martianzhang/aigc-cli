package image

import (
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runImageEditsJSON handles ZeekAI's image-to-image endpoint:
// POST /images/edits with JSON images[].image_url.
func runImageEditsJSON(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	if req.MaskURL != "" {
		return nil, fmt.Errorf("mask is not supported by the /images/edits protocol")
	}

	resolved, err := service.LocalFilesToDataURI(req.ImageURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve image-urls: %w", err)
	}
	req.ImageURLs = resolved

	start := time.Now()
	resp, err := c.ImageGenerateEdits(req)
	if err != nil {
		return nil, fmt.Errorf("image edits failed: %w", err)
	}
	elapsed := time.Since(start)

	p := options.Shared.ResolveProvider("image")
	fmt.Printf("Provider: %s\n", p.ProviderType)
	fmt.Printf("Model: %s\n", req.Model)
	if resp.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(resp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	saved, err := saveOpenAIImageResponse(resp)
	if err != nil {
		return nil, err
	}

	postProcessImages(saved)
	service.PrintUsage(resp.Usage)

	return saved, nil
}
