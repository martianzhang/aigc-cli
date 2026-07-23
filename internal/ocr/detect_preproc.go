package ocr

import (
	"image"
	"math"

	"golang.org/x/image/draw"
)

// detPreprocess resizes an image for DBNet input while maintaining aspect ratio,
// then normalizes to CHW float32 format with PP-OCR normalization  [-1, 1].
// inputSize is the padded size fed into the model (e.g., 960, 1920).
func detPreprocess(img image.Image, inputSize int) (pixels []float32, scaleX, scaleY float64, padLeft, padTop int) {
	b := img.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()

	// Resize so longest side ≤ inputSize (but don't upscale if it already fits).
	ratio := float64(inputSize) / float64(maxInt(srcW, srcH))
	if ratio > 1.0 {
		ratio = 1.0
	}
	resizedW := int(math.Round(float64(srcW) * ratio))
	resizedH := int(math.Round(float64(srcH) * ratio))

	resized := image.NewRGBA(image.Rect(0, 0, resizedW, resizedH))
	draw.BiLinear.Scale(resized, resized.Bounds(), img, b, draw.Src, nil)

	// Center-pad to inputSize x inputSize
	padLeft = (inputSize - resizedW) / 2
	padTop = (inputSize - resizedH) / 2

	// PP-OCR normalization: (pixel/255 - 0.5) / 0.5  →  [-1, 1]
	mean := [3]float32{0.5, 0.5, 0.5}
	std := [3]float32{0.5, 0.5, 0.5}

	pixels = make([]float32, DetChannels*inputSize*inputSize)
	for c := 0; c < DetChannels; c++ {
		for y := 0; y < inputSize; y++ {
			for x := 0; x < inputSize; x++ {
				ix := x - padLeft
				iy := y - padTop
				idx := c*inputSize*inputSize + y*inputSize + x
				if ix < 0 || ix >= resizedW || iy < 0 || iy >= resizedH {
					pixels[idx] = (0.0 - mean[c]) / std[c]
					continue
				}
				r, g, b_, _ := resized.At(ix, iy).RGBA()
				var val float32
				switch c {
				case 0:
					val = float32(r) / 65535.0
				case 1:
					val = float32(g) / 65535.0
				case 2:
					val = float32(b_) / 65535.0
				}
				pixels[idx] = (val - mean[c]) / std[c]
			}
		}
	}

	scaleX = float64(resizedW) / float64(inputSize)
	scaleY = float64(resizedH) / float64(inputSize)
	return pixels, scaleX, scaleY, padLeft, padTop
}
