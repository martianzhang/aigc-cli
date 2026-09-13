package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// downloadImages downloads all generated images to the output directory.
// Returns paths to saved files.
func downloadImages(images []types.ImageResult, taskID string) ([]string, error) {
	var saved []string
	for i, img := range images {
		for j, url := range img.URL {
			data, err := service.FetchBytes(url)
			if err != nil {
				// Save raw data as text file for manual recovery
				prefix := fmt.Sprintf("image_%s_%d_%d", taskID, i, j)
				service.SaveBase64Fallback(shared.OutputDir, prefix, url, 0)
				continue
			}

			// Prefer sniffed real format over URL extension (URLs often lack
			// an extension or carry query params, causing wrong file types).
			ext := imgcodec.SniffImageExt(data)
			if ext == "" {
				ext = extractImageExt(url)
			}
			if ext == "" {
				ext = ".png"
			}
			filename := filepath.Join(shared.OutputDir, fmt.Sprintf("image_%s_%d_%d%s", taskID, i, j, ext))
			if err := os.WriteFile(filename, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", filename, err)
				continue
			}
			fmt.Printf("Saved: %s\n", filename)
			saved = append(saved, filename)
		}
	}
	return saved, nil
}
