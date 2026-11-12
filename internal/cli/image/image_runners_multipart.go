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

// runImageEditsMultipart handles OpenAI-family image-to-image:
// POST /images/edits with a multipart/form-data body (image=@<file>).
func runImageEditsMultipart(c client.APIClient, req *types.GenerateRequest, ctx *imageDispatchCtx) ([]string, error) {
	start := time.Now()
	resp, err := c.ImageGenerateEditsMultipart(req)
	if err != nil {
		return nil, fmt.Errorf("image edits (multipart) failed: %w", err)
	}
	elapsed := time.Since(start)

	ui.Printf("Provider: %s\n", ctx.providerFor().ProviderType)
	ui.Printf("Model: %s\n", req.Model)
	if resp.Created > 0 {
		ui.Printf("Created: %s\n", time.Unix(resp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	ui.Printf("Duration: %.1fs\n", elapsed.Seconds())

	taskID := promptTaskID("image_sync", resp.Created)
	service.SavePromptBestEffort(options.Shared.SavePrompt, options.Shared.OutputDir, "image", taskID, req.Prompt)

	saved, err := saveOpenAIImageResponse(resp, taskID)
	if err != nil {
		return nil, err
	}

	postProcessImages(saved)
	service.PrintUsage(resp.Usage)

	return saved, nil
}
