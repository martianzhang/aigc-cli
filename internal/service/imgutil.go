package service

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
	"github.com/martianzhang/aigc-cli/internal/types"
)

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true,
	".gif": true, ".avif": true, ".heic": true, ".jxl": true,
}

// IsImageInput reports whether path has a known image extension.
func IsImageInput(path string) bool {
	return imageExts[strings.ToLower(filepath.Ext(path))]
}

// ExtractImageExt returns the lowercase image extension from a URL (query
// stripped), or "" when absent/too long.
func ExtractImageExt(rawURL string) string {
	if idx := strings.Index(rawURL, "?"); idx >= 0 {
		rawURL = rawURL[:idx]
	}
	ext := strings.ToLower(filepath.Ext(rawURL))
	if ext == "" || len(ext) > 5 {
		return ""
	}
	return ext
}

// ParseHexColor parses "#RRGGBB" or "RRGGBB" into an opaque RGBA color.
func ParseHexColor(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("color must be 6-digit hex, got %q", s)
	}
	var r, g, b uint8
	n, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	if err != nil || n != 3 {
		return color.RGBA{}, fmt.Errorf("invalid hex color %q", s)
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}, nil
}

// ParseOffset parses "dx,dy" into two ints.
func ParseOffset(s string) (int, int, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("offset must be in format \"dx,dy\", got %q", s)
	}
	var dx, dy int
	if _, err := fmt.Sscanf(parts[0], "%d", &dx); err != nil {
		return 0, 0, fmt.Errorf("invalid dx: %s", parts[0])
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &dy); err != nil {
		return 0, 0, fmt.Errorf("invalid dy: %s", parts[1])
	}
	return dx, dy, nil
}

// DownloadImages downloads all generated images into outputDir. Returns paths
// to saved files.
func DownloadImages(images []types.ImageResult, outputDir, taskID string) ([]string, error) {
	var saved []string
	for i, img := range images {
		for j, url := range img.URL {
			data, err := FetchBytes(url)
			if err != nil {
				prefix := fmt.Sprintf("image_%s_%d_%d", taskID, i, j)
				SaveBase64Fallback(outputDir, prefix, url, 0)
				continue
			}

			ext := imgcodec.SniffImageExt(data)
			if ext == "" {
				ext = ExtractImageExt(url)
			}
			if ext == "" {
				ext = ".png"
			}
			filename := filepath.Join(outputDir, fmt.Sprintf("image_%s_%d_%d%s", taskID, i, j, ext))
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
