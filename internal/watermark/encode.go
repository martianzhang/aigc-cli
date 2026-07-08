package watermark

import (
	"image"
	"image/png"
	"io"
)

func encodePNG(w io.Writer, img *image.RGBA) error {
	return png.Encode(w, img)
}

// Ensure image package is used.
var _ image.Image
