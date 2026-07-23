//go:build cgo

package service

import (
	"image"
	"io"

	"github.com/chai2010/webp"
)

func init() {
	webpEncode = webpEncodeCgo
}

func webpEncodeCgo(w io.Writer, m image.Image, quality int) error {
	return webp.Encode(w, m, &webp.Options{Quality: float32(quality), Lossless: false})
}
