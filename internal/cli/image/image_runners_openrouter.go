package image

import (
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// runOpenRouterDedicatedImage handles image generation via OpenRouter's
// dedicated Image API (POST /v1/images). Used for GPT Image, DALL-E, and
// most dedicated image models on OpenRouter. Returns standard OpenAI-compatible
// response with b64_json images.
func runOpenRouterDedicatedImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	orResp, err := c.OpenRouterDedicatedImage(req)
	if err != nil {
		return nil, fmt.Errorf("OpenRouter image generation failed: %w", err)
	}

	elapsed := time.Since(start)

	ui.Printf("Model: %s\n", req.Model)
	if orResp.Created > 0 {
		ui.Printf("Created: %s\n", time.Unix(orResp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	ui.Printf("Duration: %.1fs\n", elapsed.Seconds())

	service.SavePromptBestEffort(options.Shared.SavePrompt, options.Shared.OutputDir, "image", promptTaskID("image", orResp.Created), req.Prompt)

	var saved []string
	for i, img := range orResp.Data {
		if img.B64JSON != "" {
			prefix := fmt.Sprintf("image_%d", time.Now().Unix())
			filename, err := service.SaveBase64Image(options.Shared.OutputDir, prefix, img.B64JSON, i)
			if err != nil {
				ui.Eprintf("Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			ui.Printf("Image %d saved: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			taskID := fmt.Sprintf("%d", time.Now().Unix())
			filename, err := service.DownloadFile(img.URL, options.Shared.OutputDir, fmt.Sprintf("image_%s_%d", taskID, i))
			if err != nil {
				ui.Eprintf("Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			ui.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		}
		if img.RevisedPrompt != "" {
			ui.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	if len(saved) == 0 {
		return nil, fmt.Errorf("OpenRouter returned no images to save")
	}
	postProcessImages(saved)
	service.PrintUsage(orResp.Usage)

	return saved, nil
}
