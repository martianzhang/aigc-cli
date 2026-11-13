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

// runSyncImage handles OpenAI/OpenRouter-compatible synchronous image generation.
func runSyncImage(c client.APIClient, req *types.GenerateRequest, ctx *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	syncResp, err := c.ImageGenerateSync(req)
	if err != nil {
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	elapsed := time.Since(start)

	ui.Printf("Provider: %s\n", ctx.providerFor().ProviderType)
	ui.Printf("Model: %s\n", req.Model)
	if syncResp.Created > 0 {
		ui.Printf("Created: %s\n", time.Unix(syncResp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	ui.Printf("Duration: %.1fs\n", elapsed.Seconds())

	taskID := promptTaskID("image_sync", syncResp.Created)
	service.SavePromptBestEffort(options.Shared.SavePrompt, options.Shared.OutputDir, "image", taskID, req.Prompt)

	saved, err := saveOpenAIImageResponse(syncResp, taskID)
	if err != nil {
		return nil, err
	}

	postProcessImages(saved)
	service.PrintUsage(syncResp.Usage)

	return saved, nil
}

// saveOpenAIImageResponse saves every image in an OpenAI-style response and
// prints its path. Shared by the sync and images/edits runners. taskID comes
// from promptTaskID so it always carries a unique timestamp, even when the
// provider omits the `created` field.
func saveOpenAIImageResponse(resp *types.OpenAIImageResponse, taskID string) ([]string, error) {
	var saved []string
	for i, img := range resp.Data {
		switch {
		case img.B64JSON != "":
			filename, err := service.SaveBase64Image(options.Shared.OutputDir, taskID, img.B64JSON, i)
			if err != nil {
				ui.Eprintf("Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			ui.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		case img.URL != "":
			filename, err := service.DownloadFile(img.URL, options.Shared.OutputDir, fmt.Sprintf("%s_%d", taskID, i))
			if err != nil {
				ui.Eprintf("Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			ui.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		default:
			ui.Printf("Image %d: <no data>\n", i+1)
			continue
		}
		if img.RevisedPrompt != "" && options.Shared.Verbose {
			ui.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}
	if len(saved) == 0 {
		return nil, fmt.Errorf("no images saved")
	}
	return saved, nil
}
