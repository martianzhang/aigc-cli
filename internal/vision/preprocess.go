package vision

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// PreprocessImage decodes an image file and preprocesses it into a normalized
// CHW float32 tensor suitable for Florence-2 encoder input.
//
// Steps:
//  1. Decode image from file
//  2. Resize to InputSize×InputSize (224×224)
//  3. Convert to CHW layout
//  4. Normalize using ImageNet mean/std
func PreprocessImage(path string) ([]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	return PreprocessImageFromImage(img, InputSize)
}

// PreprocessImageFromImage preprocesses a decoded image for Florence-2.
func PreprocessImageFromImage(img image.Image, targetSize int) ([]float32, error) {
	// Resize to targetSize×targetSize
	resized := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
	srcBounds := img.Bounds()
	draw.BiLinear.Scale(resized, resized.Bounds(), img, srcBounds, draw.Src, nil)

	// Convert to CHW float32 and normalize with ImageNet stats
	pixels := make([]float32, 3*targetSize*targetSize)
	idx := 0
	for c := 0; c < 3; c++ {
		for y := 0; y < targetSize; y++ {
			for x := 0; x < targetSize; x++ {
				r, g, b, _ := resized.At(x, y).RGBA()
				var val float32
				switch c {
				case 0: // R
					val = float32(r) / 65535.0
				case 1: // G
					val = float32(g) / 65535.0
				case 2: // B
					val = float32(b) / 65535.0
				}
				// Normalize: (val - mean) / std
				val = (val - MeanRGB[c]) / StdRGB[c]
				pixels[idx] = val
				idx++
			}
		}
	}

	return pixels, nil
}
