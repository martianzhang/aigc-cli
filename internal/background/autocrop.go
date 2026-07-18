package background

import (
	"fmt"
	"math"
)

// findBounds finds the bounding box of non-transparent pixels (alpha > 0).
// Returns x0, y0, x1, y1 (inclusive).
func findBounds(alpha []uint8, w, h int) (x0, y0, x1, y1 int, ok bool) {
	x0, y0 = w, h
	x1, y1 = -1, -1

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if alpha[y*w+x] > 0 {
				if x < x0 {
					x0 = x
				}
				if x > x1 {
					x1 = x
				}
				if y < y0 {
					y0 = y
				}
				if y > y1 {
					y1 = y
				}
			}
		}
	}

	if x1 < x0 || y1 < y0 {
		return 0, 0, 0, 0, false
	}
	return x0, y0, x1, y1, true
}

// applyPadding expands the bounding box by padding pixels.
// padding order: [top, right, bottom, left]
func applyPadding(x0, y0, x1, y1, imgW, imgH int, padding [4]int) (int, int, int, int) {
	x0 -= padding[3] // left
	y0 -= padding[0] // top
	x1 += padding[1] // right
	y1 += padding[2] // bottom

	// Clamp to image bounds
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= imgW {
		x1 = imgW - 1
	}
	if y1 >= imgH {
		y1 = imgH - 1
	}

	return x0, y0, x1, y1
}

// applyAspectRatio expands the bounding box to match the target aspect ratio.
// The box is expanded outward (centered) — never shrinks.
// If aspectRatio is empty, returns the original bounds unchanged.
func applyAspectRatio(x0, y0, x1, y1, imgW, imgH int, aspectRatio string) (int, int, int, int) {
	if aspectRatio == "" {
		return x0, y0, x1, y1
	}

	// Parse "W:H"
	var targetW, targetH float64
	n, err := fmt.Sscanf(aspectRatio, "%f:%f", &targetW, &targetH)
	if err != nil || n != 2 || targetW <= 0 || targetH <= 0 {
		return x0, y0, x1, y1
	}

	curW := float64(x1 - x0 + 1)
	curH := float64(y1 - y0 + 1)

	targetRatio := targetW / targetH
	currentRatio := curW / curH

	var newW, newH float64
	if currentRatio < targetRatio {
		// Need to expand width
		newW = curH * targetRatio
		newH = curH
	} else {
		// Need to expand height
		newW = curW
		newH = curW / targetRatio
	}

	// Center the expansion
	dx := int(math.Ceil((newW - curW) / 2))
	dy := int(math.Ceil((newH - curH) / 2))

	x0 -= dx
	x1 += dx
	y0 -= dy
	y1 += dy

	// Clamp to image bounds
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= imgW {
		x1 = imgW - 1
	}
	if y1 >= imgH {
		y1 = imgH - 1
	}

	return x0, y0, x1, y1
}

// cropImage extracts a rectangular region from RGBA pixel data.
func cropImage(rgba []uint8, srcW, srcH, x0, y0, x1, y1 int) ([]uint8, int, int) {
	dstW := x1 - x0 + 1
	dstH := y1 - y0 + 1
	dst := make([]uint8, dstW*dstH*4)

	for dy := 0; dy < dstH; dy++ {
		sy := y0 + dy
		for dx := 0; dx < dstW; dx++ {
			sx := x0 + dx
			srcIdx := sy*srcW*4 + sx*4
			dstIdx := dy*dstW*4 + dx*4
			copy(dst[dstIdx:dstIdx+4], rgba[srcIdx:srcIdx+4])
		}
	}

	return dst, dstW, dstH
}
