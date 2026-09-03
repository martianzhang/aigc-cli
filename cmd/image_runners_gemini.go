package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runGeminiImage handles image generation via Gemini native generateContent API.
func runGeminiImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()

	geminiResp, err := c.GeminiImageGenerate(req)
	if err != nil {
		return nil, fmt.Errorf("gemini image generation failed: %w", err)
	}

	elapsed := time.Since(start)

	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Duration: %.1fs\n", elapsed.Seconds())

	var saved []string
	for i, img := range geminiResp.Data {
		if strings.HasPrefix(img.URL, "data:") {
			prefix := fmt.Sprintf("image_%d", time.Now().Unix())
			filename, err := service.SaveBase64Image(shared.OutputDir, prefix, img.URL, i)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save image %d: %v\n", i, err)
				continue
			}
			fmt.Printf("Image %d saved: %s\n", i+1, filename)
			saved = append(saved, filename)
		} else if img.URL != "" {
			data, err := service.FetchBytes(img.URL)
			if err != nil {
				service.SaveBase64Fallback(shared.OutputDir, fmt.Sprintf("image_%d", time.Now().Unix()), img.URL, 0)
				continue
			}
			// Prefer sniffed real format over URL extension (URLs often lack
			// an extension or carry query params, causing wrong file types).
			ext := imgcodec.SniffImageExt(data)
			if ext == "" {
				ext = extractImageExt(img.URL)
			}
			if ext == "" {
				ext = ".png"
			}
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%d_%d%s", time.Now().Unix(), i, ext))
			if err := os.WriteFile(filename, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Image %d saved: %s\n", i+1, filename)
			saved = append(saved, filename)
		}
	}

	postProcessImages(saved)
	return saved, nil
}
