package ocr

import (
	"image"
	"math"

	"golang.org/x/image/draw"
)

const angleThresholdRad = 0.05 // ~3° — below this, skip affine transform (fast path)

// boxAngle computes the rotation angle (in radians) of a text box from horizontal.
// Uses the longer of top or bottom edge for stability.
func boxAngle(box [4][2]int) float64 {
	// Compute widths of top and bottom edges
	dx1 := float64(box[1][0] - box[0][0])
	dy1 := float64(box[1][1] - box[0][1])
	dx2 := float64(box[2][0] - box[3][0])
	dy2 := float64(box[2][1] - box[3][1])
	w1 := math.Hypot(dx1, dy1)
	w2 := math.Hypot(dx2, dy2)
	var angle float64
	if w1 >= w2 {
		angle = math.Atan2(dy1, dx1)
	} else {
		angle = math.Atan2(dy2, dx2)
	}
	// Normalize to [-pi/2, pi/2)
	if angle < -math.Pi/2 {
		angle += math.Pi
	} else if angle >= math.Pi/2 {
		angle -= math.Pi
	}
	return angle
}

// cropTextLine extracts a text region from the image, applying affine
// transformation to straighten the text line to horizontal.
// Works with oriented boxes from DBNet.
func cropTextLine(img image.Image, box [4][2]int) *image.RGBA {
	b := img.Bounds()
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
		return nil
	}

	// Compute text line angle.
	angle := boxAngle(box)

	// Fast path: near-horizontal text, just do axis-aligned crop.
	if math.Abs(angle) <= angleThresholdRad {
		cropped := image.NewRGBA(image.Rect(0, 0, cropW, cropH))
		for y := 0; y < cropH; y++ {
			for x := 0; x < cropW; x++ {
				cropped.Set(x, y, img.At(minX+x, minY+y))
			}
		}
		return cropped
	}

	// Affine path: rotate the crop to make the text horizontal.
	// Compute the center of the cropped region.
	cx := float64(cropW) / 2
	cy := float64(cropH) / 2

	// Compute the bounding rect of the rotated crop so we don't clip.
	cosA := math.Cos(-angle)
	sinA := math.Sin(-angle)
	corners := [][2]float64{
		{-cx, -cy}, {float64(cropW) - cx, -cy},
		{float64(cropW) - cx, float64(cropH) - cy}, {-cx, float64(cropH) - cy},
	}
	minDstX, maxDstX := 0.0, 0.0
	minDstY, maxDstY := 0.0, 0.0
	for _, c := range corners {
		rx := c[0]*cosA - c[1]*sinA
		ry := c[0]*sinA + c[1]*cosA
		if rx < minDstX {
			minDstX = rx
		}
		if rx > maxDstX {
			maxDstX = rx
		}
		if ry < minDstY {
			minDstY = ry
		}
		if ry > maxDstY {
			maxDstY = ry
		}
	}
	dstW := int(math.Ceil(maxDstX - minDstX))
	dstH := int(math.Ceil(maxDstY - minDstY))
	if dstW < 1 || dstH < 1 {
		return nil
	}

	// Draw rotated image using inverse mapping.
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	cosNA := math.Cos(angle)
	sinNA := math.Sin(angle)
	offX := cx + float64(minX) - minDstX
	offY := cy + float64(minY) - minDstY
	for dy := 0; dy < dstH; dy++ {
		for dx := 0; dx < dstW; dx++ {
			// Inverse map: destination (dx,dy) → source (sx,sy)
			sx := (float64(dx)+minDstX)*cosNA - (float64(dy)+minDstY)*sinNA + offX
			sy := (float64(dx)+minDstX)*sinNA + (float64(dy)+minDstY)*cosNA + offY
			ix, iy := int(math.Round(sx)), int(math.Round(sy))
			if ix >= b.Min.X && ix < b.Max.X && iy >= b.Min.Y && iy < b.Max.Y {
				dst.Set(dx, dy, img.At(ix, iy))
			}
		}
	}
	return dst
}

// prepareRecInput takes a cropped text image, resizes to fixed height,
// pads to RecMaxWidth width, and normalizes to PP-OCR format.
// Returns normalized CHW float32 pixels and the actual width used (before padding).
func prepareRecInput(cropped *image.RGBA) (pixels []float32, regionWidth int) {
	if cropped == nil {
		return nil, 0
	}
	cropW := cropped.Bounds().Dx()
	cropH := cropped.Bounds().Dy()
	if cropW <= 0 || cropH <= 0 {
		return nil, 0
	}

	ratio := float64(RecHeight) / float64(cropH)
	resizedW := int(math.Round(float64(cropW) * ratio))
	if resizedW < 1 {
		resizedW = 1
	}
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
					pixels[idx] = (0.0 - 0.5) / 0.5
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
				pixels[idx] = (val - 0.5) / 0.5
			}
		}
	}

	return pixels, resizedW
}

// recPreprocess crops a text region from the image (via affine correction),
// optionally classifies direction and rotates, then prepares the recognition input.
func recPreprocess(e *Engine, img image.Image, box [4][2]int) (pixels []float32, regionWidth int) {
	cropped := cropTextLine(img, box)
	if cropped == nil {
		return nil, 0
	}

	// Direction classification: if the text is upside-down, rotate 180°.
	if e != nil && e.cls != nil && e.classifyDirection(cropped) {
		b := cropped.Bounds()
		rotated := image.NewRGBA(b)
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				rotated.Set(b.Dx()-1-x, b.Dy()-1-y, cropped.At(x, y))
			}
		}
		cropped = rotated
	}

	return prepareRecInput(cropped)
}
