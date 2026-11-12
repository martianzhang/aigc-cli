package mcp

import (
	"image"
	"os"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

// decodeImageGuarded opens path and decodes it through imgcodec, which rejects
// images whose headers claim more than DefaultMaxDecodePixels (100 MP). A
// crafted header can claim enormous dimensions while carrying one pixel of
// data; decoding such a "decode bomb" would allocate gigabytes.
func decodeImageGuarded(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return imgcodec.Decode(f)
}
