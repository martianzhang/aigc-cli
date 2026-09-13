package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/service"
)

// runLocalDecode implements the pure local decode/convert mode (no API call).
// Decodes base64 text files (data URI / raw base64) to real images, and/or
// converts image format via --output-format, saving results to the output dir.
func runLocalDecode(imageURLs []string, targetFormat string) error {
	if len(imageURLs) == 0 {
		return fmt.Errorf("--image-url is required for local decode mode")
	}

	outDir := shared.OutputDir
	if outDir == "" {
		outDir = "."
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	var results []string
	for _, src := range imageURLs {
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			fmt.Fprintf(os.Stderr, "Warning: skipping remote URL (local files only): %s\n", src)
			continue
		}
		if !isFile(src) && !strings.HasPrefix(src, "data:") {
			fmt.Fprintf(os.Stderr, "Warning: file not found: %s\n", src)
			continue
		}

		dest, created, err := service.ResolveImageSource(src, targetFormat, outDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: decode %s: %v\n", src, err)
			continue
		}
		if !created {
			fmt.Fprintf(os.Stderr, "Warning: %s: no conversion needed\n", src)
			continue
		}
		results = append(results, dest)
	}

	if len(results) == 0 {
		return fmt.Errorf("no files were decoded")
	}
	fmt.Println("Decode results:")
	for _, r := range results {
		fmt.Printf("  %s\n", r)
		if genPreview {
			if e := service.PreviewFile(r); e != nil {
				fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
			}
		}
	}
	return nil
}
