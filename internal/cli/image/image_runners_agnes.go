package image

import (
	"fmt"
	"os"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// prepareAgnesImageRequest moves Agnes-specific fields into extra_body and
// embeds local input images as data URIs (Agnes has no upload endpoint).
func prepareAgnesImageRequest(req *types.GenerateRequest) error {
	// Transform ImageURLs into extra_body.image (Agnes requires it nested).
	// Agnes has no upload endpoint, so local files become data URIs first.
	if len(req.ImageURLs) > 0 {
		resolved, err := service.LocalFilesToDataURI(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		if req.ExtraBody == nil {
			req.ExtraBody = make(map[string]interface{})
		}
		req.ExtraBody["image"] = resolved
		req.ImageURLs = nil // clear top-level, Agnes rejects it there
	}
	// Move ratio into extra_body for consistency with Agnes API structure.
	if req.Ratio != "" {
		if req.ExtraBody == nil {
			req.ExtraBody = make(map[string]interface{})
		}
		req.ExtraBody["ratio"] = req.Ratio
	}
	// Move response_format into extra_body (Agnes rejects it at top level).
	if req.ResponseFormat != "" {
		if req.ExtraBody == nil {
			req.ExtraBody = make(map[string]interface{})
		}
		req.ExtraBody["response_format"] = req.ResponseFormat
		req.ResponseFormat = ""
	}
	// Agnes text-image queue does not support the quality parameter; drop it.
	req.Quality = ""
	// Agnes text-image queue does not support output_format; drop it.
	req.OutputFormat = ""
	return nil
}

// runAgnesImage handles image generation via the Agnes API.
// Key differences from standard OpenAI:
//   - image URLs go in extra_body.image (not top-level image_urls)
//   - response_format must be in extra_body (handled in image.go transform)
//   - 2.1 Flash supports ratio for tiered sizing (e.g. "2K" + "16:9")
func runAgnesImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	if err := prepareAgnesImageRequest(req); err != nil {
		return nil, err
	}

	start := time.Now()
	syncResp, err := c.ImageGenerateSync(req)
	if err != nil {
		return nil, fmt.Errorf("agnes image generation failed: %w", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("Model: %s\n", req.Model)
	if syncResp.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(syncResp.Created, 0).Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	var saved []string
	for i, img := range syncResp.Data {
		if img.B64JSON != "" {
			taskID := fmt.Sprintf("agnes_%d", syncResp.Created)
			filename, err := service.SaveBase64Image(options.Shared.OutputDir, taskID, img.B64JSON, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			taskID := fmt.Sprintf("agnes_%d", syncResp.Created)
			filename, err := service.DownloadFile(img.URL, options.Shared.OutputDir, fmt.Sprintf("image_%s_%d", taskID, i))
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
		if img.RevisedPrompt != "" {
			fmt.Printf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	postProcessImages(saved)
	service.PrintUsage(syncResp.Usage)
	return saved, nil
}
