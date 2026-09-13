package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/service"
)

// runLocalCompress implements the pure local compression mode (no API call).
// Compress --image-url files directly using --compress settings.
func runLocalCompress(compressVal string, imageURLs []string, outputFormat string) error {
	if len(imageURLs) == 0 {
		return fmt.Errorf("--image-url is required for local compression mode")
	}
	targetSize, quality, err := service.ParseCompressOption(compressVal)
	if err != nil {
		return fmt.Errorf("invalid --compress value %q: %w", compressVal, err)
	}
	opts := &service.CompressOptions{
		TargetSize: targetSize,
		Quality:    quality,
		Format:     outputFormat,
	}
	var results []*service.CompressResult
	for _, src := range imageURLs {
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			fmt.Fprintf(os.Stderr, "Warning: skipping remote URL (local files only): %s\n", src)
			continue
		}
		if !isFile(src) {
			fmt.Fprintf(os.Stderr, "Warning: file not found: %s\n", src)
			continue
		}
		result, err := service.CompressImage(src, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: compress %s: %v\n", src, err)
			continue
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		return fmt.Errorf("no files were compressed")
	}
	fmt.Println("Compression results:")
	var totalBefore, totalAfter int64
	for _, r := range results {
		if r.Skipped {
			fmt.Printf("  %s: skipped (%s)\n", r.DstPath, r.Reason)
		} else {
			pct := 100 - int(float64(r.After)/float64(r.Before)*100)
			params := formatParams(r.Format, r.Quality)
			fmt.Printf("  %s: %s → %s (%d%% saved)%s\n", r.DstPath, formatBytes(r.Before), formatBytes(r.After), pct, params)
		}
		totalBefore += r.Before
		totalAfter += r.After
	}
	if totalBefore > 0 {
		pct := 100 - int(float64(totalAfter)/float64(totalBefore)*100)
		first := results[0]
		params := formatParams(first.Format, first.Quality)
		fmt.Printf("Total: %s → %s (%d%% saved)%s\n", formatBytes(totalBefore), formatBytes(totalAfter), pct, params)
	}
	return nil
}

// formatParams appends format and quality info for display.
func formatParams(fmtStr string, quality int) string {
	if fmtStr == "" {
		return ""
	}
	if quality > 0 {
		return fmt.Sprintf(" [%s q%d]", fmtStr, quality)
	}
	return fmt.Sprintf(" [%s]", fmtStr)
}

// formatBytes returns a human-readable byte size string.
func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
