package image

import (
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runImageEditsMultipart handles OpenAI-family image-to-image:
// POST /images/edits with a multipart/form-data body (image=@<file>).
func runImageEditsMultipart(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()
	resp, err := c.ImageGenerateEditsMultipart(req)
	if err != nil {
		return nil, fmt.Errorf("image edits (multipart) failed: %w", err)
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
