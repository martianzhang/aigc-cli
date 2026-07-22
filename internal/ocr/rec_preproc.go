package ocr

import (
	"image"
	"math"

	"golang.org/x/image/draw"
)

// recPreprocess crops a text region from the image, resizes to fixed height
// while maintaining aspect ratio, pads to RecMaxWidth width, and normalizes.
// Returns normalized CHW float32 pixels and the actual width used (before padding).
//
// The CRNN model expects [1, 3, 48, W] with simple normalization (pixel/255).
func recPreprocess(img image.Image, box [4][2]int) (pixels []float32, regionWidth int) {
	// Get bounding rect of the polygon
	minX, maxX := box[0][0], box[0][0]
	minY, maxY := box[0][1], box[0][1]
	for _, p := range box[1:] {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}

	// Clamp to image bounds
	b := img.Bounds()
	if minX < b.Min.X {
		minX = b.Min.X
	}
	if minY < b.Min.Y {
		minY = b.Min.Y
	}
	if maxX > b.Max.X {
		maxX = b.Max.X
	}
	if maxY > b.Max.Y {
		maxY = b.Max.Y
	}

	cropW := maxX - minX
	cropH := maxY - minY
	if cropW <= 0 || cropH <= 0 {
		return nil, 0
	}

	// Crop the region
	cropped := image.NewRGBA(image.Rect(0, 0, cropW, cropH))
	for y := 0; y < cropH; y++ {
		for x := 0; x < cropW; x++ {
			cropped.Set(x, y, img.At(minX+x, minY+y))
		}
	}

	// Resize to fixed height, maintaining aspect ratio
	ratio := float64(RecHeight) / float64(cropH)
	resizedW := int(math.Round(float64(cropW) * ratio))
	if resizedW < 1 {
		resizedW = 1
	}
	// Clamp to max width
	if resizedW > RecMaxWidth {
		resizedW = RecMaxWidth
	}

	resized := image.NewRGBA(image.Rect(0, 0, resizedW, RecHeight))
	draw.BiLinear.Scale(resized, resized.Bounds(), cropped, cropped.Bounds(), draw.Src, nil)

	// Build CHW tensor with PP-OCR normalization (pixel/255 → mean 0.5 → std 0.5 → [-1, 1])
	pixels = make([]float32, DetChannels*RecHeight*RecMaxWidth)
	for c := 0; c < DetChannels; c++ {
		for y := 0; y < RecHeight; y++ {
			for x := 0; x < RecMaxWidth; x++ {
				idx := c*RecHeight*RecMaxWidth + y*RecMaxWidth + x
				if x >= resizedW {
					pixels[idx] = (0.0 - 0.5) / 0.5 // zero-pad in [-1,1] space = -1
					continue
				}
				r, g, b_, _ := resized.At(x, y).RGBA()
				var val float32
				switch c {
				case 0:
					val = float32(r) / 65535.0
				case 1:
					val = float32(g) / 65535.0
				case 2:
					val = float32(b_) / 65535.0
				}
				pixels[idx] = (val - 0.5) / 0.5 // normalize to [-1, 1]
			}
		}
	}

	return pixels, resizedW
}
