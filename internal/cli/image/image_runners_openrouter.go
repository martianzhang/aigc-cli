package image

import (
	"fmt"
	"strconv"
	"strings"
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
func runOpenRouterDedicatedImage(c client.APIClient, req *types.GenerateRequest, ctx *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	filtered, err := openRouterCapabilityFiltered(req, ctx)
	if err != nil {
		return nil, err
	}

	orResp, err := c.OpenRouterDedicatedImage(filtered)
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
	printImageSummary(ctx, req, saved, "", usageCost(orResp.Usage))

	return saved, nil
}

// openRouterCapabilityFiltered drops request fields the model does not declare
// in supported_parameters. The lookup is best-effort: an unreachable discovery
// endpoint leaves the request unchanged, and the API then reports the problem.
func openRouterCapabilityFiltered(req *types.GenerateRequest, ctx *imageDispatchCtx) (*types.GenerateRequest, error) {
	p := ctx.providerFor()
	params, ok := client.OpenRouterImageParams(p.BaseURL, req.Model)
	if !ok {
		return req, nil
	}
	filtered, err := client.FilterOpenRouterImageRequest(req, params)
	if err != nil {
		return nil, err
	}
	if options.Shared.Verbose {
		logDroppedOpenRouterFields(req, filtered)
	}
	return filtered, nil
}

// logDroppedOpenRouterFields reports, in verbose mode only, which fields the
// capability filter removed so -v explains why a flag had no effect.
func logDroppedOpenRouterFields(before, after *types.GenerateRequest) {
	var dropped []string
	add := func(name, value string) {
		if value != "" {
			dropped = append(dropped, name+"="+value)
		}
	}
	if before.Size != after.Size {
		add("size", before.Size)
	}
	if before.Ratio != after.Ratio {
		add("aspect_ratio", before.Ratio)
	}
	if before.Quality != after.Quality {
		add("quality", before.Quality)
	}
	if before.OutputFormat != after.OutputFormat {
		add("output_format", before.OutputFormat)
	}
	if before.Background != after.Background {
		add("background", before.Background)
	}
	if before.Resolution != after.Resolution {
		add("resolution", before.Resolution)
	}
	if before.N != nil && after.N == nil {
		add("n", strconv.Itoa(*before.N))
	}
	if len(dropped) > 0 {
		ui.Eprintf("[image] openrouter: dropped unsupported %s\n", strings.Join(dropped, ", "))
	}
}
