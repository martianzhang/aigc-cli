package service

import (
	"fmt"
	"image"
	"io"
)

// webpEncode is set at init time by either the CGO or stub variant.
// It encodes the image as WebP lossy to w with the given quality (1-100).
var webpEncode func(w io.Writer, m image.Image, quality int) error

// init checks that webpEncode was initialized.
func init() {
	if webpEncode == nil {
		webpEncode = func(w io.Writer, m image.Image, quality int) error {
			return fmt.Errorf("WebP encoding requires CGO: install a C compiler or use --output-format jpg/png instead")
		}
	}
}
