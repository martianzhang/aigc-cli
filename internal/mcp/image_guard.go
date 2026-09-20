package mcp

import (
	"fmt"
	"image"
	"io"
	"os"
)

// maxDecodePixels caps the decoded pixel count of MCP image inputs (100 MP).
// A crafted header can claim enormous dimensions while carrying one pixel of
// data; decoding such a "decode bomb" would allocate gigabytes.
const maxDecodePixels = 100_000_000

// decodeImageGuarded opens path, rejects headers claiming more than
// maxDecodePixels, then decodes the image. Decoders are registered by the
// importers in this package (png/jpeg/gif/bmp/webp).
func decodeImageGuarded(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, "", fmt.Errorf("decode config: %w", err)
	}
	if cfg.Width*cfg.Height > maxDecodePixels {
		return nil, "", fmt.Errorf("image too large: %dx%d", cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", fmt.Errorf("rewind %s: %w", path, err)
	}

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, "", err
	}
	return img, format, nil
}
