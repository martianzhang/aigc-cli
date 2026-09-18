package video

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runPollinationsVideo handles video generation via pollinations.ai's
// synchronous GET /video/{prompt} endpoint, which returns raw MP4 bytes.
//
// Billing follows the model's advertised price: official models are paidOnly
// and need paid Pollen, while community models can also be priced (a model is
// free only when its price is zero).
func runPollinationsVideo(req *types.VideoGenerateRequest) ([]string, error) {
	if req.Prompt == "" {
		return nil, fmt.Errorf("prompt is required for pollinations video generation")
	}

	c := options.NewClient("video")
	options.ApplyTimeout(c, "video", client.VideoTimeout)

	fmt.Printf("Provider: %s\n", options.Shared.ResolveProvider("video").ProviderType)
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Println("Requesting video (pollinations generates synchronously; this may take a few minutes)...")

	start := time.Now()
	data, contentType, err := c.PollinationsVideoGenerate(req)
	if err != nil {
		return nil, fmt.Errorf("pollinations video generation failed: %w", err)
	}
	elapsed := time.Since(start).Seconds()

	ext := ".mp4"
	switch {
	case strings.Contains(contentType, "webm"):
		ext = ".webm"
	case strings.Contains(contentType, "quicktime"):
		ext = ".mov"
	}

	filename := filepath.Join(options.Shared.OutputDir, fmt.Sprintf("video_pollinations_%d%s", time.Now().Unix(), ext))
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return nil, fmt.Errorf("failed to save video: %w", err)
	}

	fmt.Printf("Size: %d bytes\n", len(data))
	fmt.Printf("Saved: %s\n", filename)
	fmt.Printf("Completed in %.0fs\n", elapsed)
	return []string{filename}, nil
}
