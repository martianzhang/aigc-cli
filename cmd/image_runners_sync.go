package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runSyncImage handles OpenAI/OpenRouter-compatible synchronous image generation.
func runSyncImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	syncResp, err := c.ImageGenerateSync(req)
	if err != nil {
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	elapsed := time.Since(start)

	p := shared.ResolveProvider("image")
	fmt.Printf("Provider: %s\n", p.ProviderType)
	fmt.Printf("Model: %s\n", req.Model)
	if syncResp.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(syncResp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())
	var saved []string
	for i, img := range syncResp.Data {
		if img.B64JSON != "" {
			taskID := fmt.Sprintf("image_sync_%d", syncResp.Created)
			filename, err := service.SaveBase64Image(shared.OutputDir, taskID, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			taskID := fmt.Sprintf("sync_%d", syncResp.Created)
			filename, err := service.DownloadFile(img.URL, shared.OutputDir, fmt.Sprintf("image_%s_%d", taskID, i))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else {
			fmt.Printf("Image %d: <no data>\n", i+1)
			continue
		}
		if img.RevisedPrompt != "" && shared.Verbose {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	postProcessImages(saved)
	printUsage(syncResp.Usage)

	return saved, nil
}
