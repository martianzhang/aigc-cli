package service

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	"golang.org/x/image/draw"

	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

// ReadImageFile reads and validates a local image file, returning its bytes.
func ReadImageFile(path string) ([]byte, error) {
	data, _, err := readLocalImage(path)
	return data, err
}

// ResizeImageBytes downscales data so its longest edge is at most maxEdge,
// preserving the aspect ratio and the source format. Images already within
// maxEdge are returned unchanged (resized=false); it never upscales. A
// maxEdge <= 0 disables resizing.
//
// Pixel dimensions, not file size, drive how many vision tokens a model sees:
// a 2848x1600 screenshot costs ~4000 tokens, the same image at 1024px ~800.
func ResizeImageBytes(data []byte, maxEdge int) (out []byte, resized bool, err error) {
	if maxEdge <= 0 {
		return data, false, nil
	}
	img, err := decodeImageSafe(data)
	if err != nil {
		return nil, false, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	longest := w
	if h > longest {
		longest = h
	}
	if longest <= maxEdge {
		return data, false, nil
	}

	scale := float64(maxEdge) / float64(longest)
	nw, nh := int(float64(w)*scale+0.5), int(float64(h)*scale+0.5)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if err := encodeResized(&buf, dst, data); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

// decodeImageSafe decodes image bytes, converting a decoder panic into an
// error. The jpegli wasm decoder panics on some valid progressive JPEGs.
func decodeImageSafe(data []byte) (img image.Image, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("image decoder panicked: %v", rec)
		}
	}()
	img, _, err = image.Decode(bytes.NewReader(data))
	return img, err
}

// encodeResized re-encodes a resized image, keeping the source format so a
// lossless PNG screenshot stays lossless. Unknown formats fall back to JPEG.
func encodeResized(buf *bytes.Buffer, img image.Image, src []byte) error {
	switch imgcodec.SniffImageExt(src) {
	case ".png":
		return png.Encode(buf, img)
	case ".jpg", ".jpeg":
		_, err := imgcodec.Encode(buf, img, "jpg", 85)
		return err
	case ".webp":
		_, err := imgcodec.Encode(buf, img, "webp", 85)
		return err
	default:
		_, err := imgcodec.Encode(buf, img, "jpg", 85)
		return err
	}
}
