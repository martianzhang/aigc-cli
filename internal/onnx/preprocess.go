package onnx

import (
	"image"
	"math"

	"golang.org/x/image/draw"
)

// Preprocess converts an image to a normalized float32 tensor suitable for
// the AIGC detection model. It resizes to targetSize x targetSize and
// normalizes pixel values to [0, 1].
func Preprocess(img image.Image, targetSize int) []float32 {
	// Resize to targetSize x targetSize using bilinear interpolation
	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	resized := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
	if srcW == targetSize && srcH == targetSize {
		// Already the right size, just copy
		draw.Copy(resized, image.Point{}, img, srcBounds, draw.Src, nil)
	} else {
		draw.BiLinear.Scale(resized, resized.Bounds(), img, srcBounds, draw.Src, nil)
	}

	// Convert to CHW float32 format and normalize to [0, 1]
	pixels := make([]float32, 3*targetSize*targetSize)
	idx := 0
	for c := 0; c < 3; c++ {
		for y := 0; y < targetSize; y++ {
			for x := 0; x < targetSize; x++ {
				r, g, b, _ := resized.At(x, y).RGBA()
				switch c {
				case 0:
					pixels[idx] = float32(r) / 65535.0 // Red channel
				case 1:
					pixels[idx] = float32(g) / 65535.0 // Green channel
				case 2:
					pixels[idx] = float32(b) / 65535.0 // Blue channel
				}
				idx++
			}
		}
	}

	return pixels
}

// Softmax applies the softmax function to the input logits.
// Returns probabilities that sum to 1.0.
func Softmax(logits []float32) []float32 {
	result := make([]float32, len(logits))
	var max float32 = logits[0]
	for _, v := range logits[1:] {
		if v > max {
			max = v
		}
	}
	var sum float64
	for i, v := range logits {
		result[i] = float32(math.Exp(float64(v - max)))
		sum += float64(result[i])
	}
	for i := range result {
		result[i] = float32(float64(result[i]) / sum)
	}
	return result
}
