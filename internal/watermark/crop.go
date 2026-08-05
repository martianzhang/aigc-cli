package watermark

import (
	"image"
	"math"
)

// CropRegion represents a detected watermark region for cropping purposes.
type CropRegion struct {
	X, Y, W, H int     // bounding box
	Confidence float64 // detection confidence
	Method     string  // detection method used
}

// DetectWatermarkRegions performs generic watermark region detection without
// requiring pre-learned templates. Uses edge density and brightness anomaly
// detection to find potential watermark areas in image corners.
func DetectWatermarkRegions(img image.Image) []CropRegion {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 64 || h < 64 {
		return nil
	}

	gray := toGrayscale(img, w, h)
	grad := sobelMagnitude(gray, w, h)

	var regions []CropRegion

	// Method 1: Edge density in corners
	edgeRegions := detectEdgeDensityInCorners(gray, grad, w, h)
	regions = append(regions, edgeRegions...)

	// Method 2: Brightness anomaly in corners
	brightnessRegions := detectBrightnessAnomalyInCorners(gray, w, h)
	regions = append(regions, brightnessRegions...)

	// Merge overlapping regions
	return mergeCropRegions(regions)
}

// detectEdgeDensityInCorners detects regions with high edge density in corners.
// Watermarks typically have dense edges (text/graphic outlines).
func detectEdgeDensityInCorners(gray, grad []float64, w, h int) []CropRegion {
	blockSize := 32
	margin := float64(minInt(w, h)) * 0.15 // corners = 15% of image

	var regions []CropRegion

	// Only scan corner regions
	corners := []struct {
		startX, startY, endX, endY int
		name                       string
	}{
		{0, 0, int(margin), int(margin), "top-left"},
		{w - int(margin), 0, w, int(margin), "top-right"},
		{0, h - int(margin), int(margin), h, "bottom-left"},
		{w - int(margin), h - int(margin), w, h, "bottom-right"},
	}

	// Compute global edge density
	globalEdgeSum := 0.0
	globalCount := 0
	for y := 0; y < h; y += 2 { // sample every 2 pixels for speed
		for x := 0; x < w; x += 2 {
			globalEdgeSum += grad[y*w+x]
			globalCount++
		}
	}
	globalEdgeMean := globalEdgeSum / float64(globalCount)

	for _, corner := range corners {
		// Scan blocks in this corner
		for by := corner.startY; by < corner.endY; by += blockSize {
			for bx := corner.startX; bx < corner.endX; bx += blockSize {
				endX := minInt(bx+blockSize, w)
				endY := minInt(by+blockSize, h)

				// Compute block edge density
				blockSum := 0.0
				blockCount := 0
				for y := by; y < endY; y++ {
					for x := bx; x < endX; x++ {
						blockSum += grad[y*w+x]
						blockCount++
					}
				}
				blockMean := blockSum / float64(blockCount)

				// Watermark region typically has 3-5x higher edge density
				if globalEdgeMean > 0 && blockMean > globalEdgeMean*3.0 {
					confidence := math.Min(1.0, blockMean/(globalEdgeMean*5.0))
					if confidence > 0.3 { // higher threshold
						regions = append(regions, CropRegion{
							X:          bx,
							Y:          by,
							W:          endX - bx,
							H:          endY - by,
							Confidence: confidence,
							Method:     "edge_density",
						})
					}
				}
			}
		}
	}

	return regions
}

// detectBrightnessAnomalyInCorners detects regions with abnormal brightness in corners.
// Watermarks (especially white/gray text) often create brightness anomalies.
func detectBrightnessAnomalyInCorners(gray []float64, w, h int) []CropRegion {
	blockSize := 32
	margin := float64(minInt(w, h)) * 0.15

	// Compute global mean brightness
	globalSum := 0.0
	globalCount := 0
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x += 2 {
			globalSum += gray[y*w+x]
			globalCount++
		}
	}
	globalMean := globalSum / float64(globalCount)

	var regions []CropRegion

	corners := []struct {
		startX, startY, endX, endY int
	}{
		{0, 0, int(margin), int(margin)},
		{w - int(margin), 0, w, int(margin)},
		{0, h - int(margin), int(margin), h},
		{w - int(margin), h - int(margin), w, h},
	}

	for _, corner := range corners {
		for by := corner.startY; by < corner.endY; by += blockSize {
			for bx := corner.startX; bx < corner.endX; bx += blockSize {
				endX := minInt(bx+blockSize, w)
				endY := minInt(by+blockSize, h)

				// Compute block mean brightness
				blockSum := 0.0
				blockCount := 0
				for y := by; y < endY; y++ {
					for x := bx; x < endX; x++ {
						blockSum += gray[y*w+x]
						blockCount++
					}
				}
				blockMean := blockSum / float64(blockCount)

				// Check for brightness anomaly (>25% deviation, stricter threshold)
				deviation := math.Abs(blockMean-globalMean) / math.Max(globalMean, 0.01)
				if deviation > 0.25 {
					confidence := math.Min(1.0, deviation/0.8)
					if confidence > 0.3 { // higher threshold
						regions = append(regions, CropRegion{
							X:          bx,
							Y:          by,
							W:          endX - bx,
							H:          endY - by,
							Confidence: confidence,
							Method:     "brightness",
						})
					}
				}
			}
		}
	}

	return regions
}

// mergeCropRegions merges overlapping regions and returns the best ones.
func mergeCropRegions(regions []CropRegion) []CropRegion {
	if len(regions) == 0 {
		return nil
	}

	// Simple approach: group by proximity and merge
	merged := make([]CropRegion, 0, len(regions))
	used := make([]bool, len(regions))

	for i := range regions {
		if used[i] {
			continue
		}

		group := CropRegion{
			X:          regions[i].X,
			Y:          regions[i].Y,
			W:          regions[i].W,
			H:          regions[i].H,
			Confidence: regions[i].Confidence,
		}

		for j := i + 1; j < len(regions); j++ {
			if used[j] {
				continue
			}

			// Check if regions overlap or are adjacent
			if regionsOverlap(group, regions[j]) {
				// Merge: expand bounding box
				newX := minInt(group.X, regions[j].X)
				newY := minInt(group.Y, regions[j].Y)
				newRight := maxInt(group.X+group.W, regions[j].X+regions[j].W)
				newBottom := maxInt(group.Y+group.H, regions[j].Y+regions[j].H)

				group.X = newX
				group.Y = newY
				group.W = newRight - newX
				group.H = newBottom - newY
				group.Confidence = math.Max(group.Confidence, regions[j].Confidence)
				used[j] = true
			}
		}

		merged = append(merged, group)
	}

	// Sort by confidence descending
	for i := 0; i < len(merged); i++ {
		for j := i + 1; j < len(merged); j++ {
			if merged[j].Confidence > merged[i].Confidence {
				merged[i], merged[j] = merged[j], merged[i]
			}
		}
	}

	return merged
}

// regionsOverlap checks if two regions overlap or are adjacent.
func regionsOverlap(a, b CropRegion) bool {
	const gap = 16 // allow 16px gap for adjacency
	return a.X-gap < b.X+b.W && a.X+a.W+gap > b.X &&
		a.Y-gap < b.Y+b.H && a.Y+a.H+gap > b.Y
}

// CropBounds represents the computed crop area.
type CropBounds struct {
	X, Y, W, H int  // crop rectangle
	Valid      bool // false if watermark is in center (can't crop)
}

const maxCropRatio = 0.20 // maximum 20% crop allowed

// ComputeCropBounds calculates the minimum crop area to remove watermarks.
// Returns invalid bounds if watermark is in the center or crop is too large.
func ComputeCropBounds(imgW, imgH int, regions []CropRegion) CropBounds {
	if len(regions) == 0 {
		return CropBounds{X: 0, Y: 0, W: imgW, H: imgH, Valid: true}
	}

	// Find the bounding box of all watermark regions
	minX, minY := imgW, imgH
	maxX, maxY := 0, 0

	for _, r := range regions {
		if r.X < minX {
			minX = r.X
		}
		if r.Y < minY {
			minY = r.Y
		}
		if r.X+r.W > maxX {
			maxX = r.X + r.W
		}
		if r.Y+r.H > maxY {
			maxY = r.Y + r.H
		}
	}

	// Add margin to ensure complete removal
	const margin = 10

	// Determine which corners contain watermarks
	inRight := maxX > int(float64(imgW)*0.7)
	inBottom := maxY > int(float64(imgH)*0.7)
	inLeft := minX < int(float64(imgW)*0.3)
	inTop := minY < int(float64(imgH)*0.3)

	var cropW, cropH, cropX, cropY int

	// Calculate crop bounds based on watermark position
	switch {
	case inRight && inBottom: // bottom-right
		cropX, cropY = 0, 0
		cropW = minX - margin
		cropH = minY - margin

	case inLeft && inBottom: // bottom-left
		cropX = maxX + margin
		cropY = 0
		cropW = imgW - cropX
		cropH = minY - margin

	case inRight && inTop: // top-right
		cropX = 0
		cropY = maxY + margin
		cropW = minX - margin
		cropH = imgH - cropY

	case inLeft && inTop: // top-left
		cropX = maxX + margin
		cropY = maxY + margin
		cropW = imgW - cropX
		cropH = imgH - cropY

	case inRight: // right side
		cropX, cropY = 0, 0
		cropW = minX - margin
		cropH = imgH

	case inBottom: // bottom side
		cropX, cropY = 0, 0
		cropW = imgW
		cropH = minY - margin

	case inLeft: // left side
		cropX = maxX + margin
		cropY = 0
		cropW = imgW - cropX
		cropH = imgH

	case inTop: // top side
		cropX = 0
		cropY = maxY + margin
		cropW = imgW
		cropH = imgH - cropY

	default: // center - can't use cropping
		return CropBounds{Valid: false}
	}

	// Check minimum size
	if cropW < 100 || cropH < 100 {
		return CropBounds{Valid: false}
	}

	// Check if crop is too large (>20% of image)
	cropRatioW := float64(imgW-cropW) / float64(imgW)
	cropRatioH := float64(imgH-cropH) / float64(imgH)
	if cropRatioW > maxCropRatio || cropRatioH > maxCropRatio {
		return CropBounds{Valid: false}
	}

	return CropBounds{X: cropX, Y: cropY, W: cropW, H: cropH, Valid: true}
}

// AdjustCropToTarget adjusts crop bounds to match target dimensions while
// maintaining aspect ratio. The crop is centered on the original crop area.
func AdjustCropToTarget(bounds CropBounds, targetW, targetH int) CropBounds {
	if !bounds.Valid || targetW <= 0 || targetH <= 0 {
		return bounds
	}

	targetRatio := float64(targetW) / float64(targetH)
	currentRatio := float64(bounds.W) / float64(bounds.H)

	// Already close enough
	if math.Abs(currentRatio-targetRatio) < 0.01 {
		return bounds
	}

	centerX := bounds.X + bounds.W/2
	centerY := bounds.Y + bounds.H/2

	var newW, newH int
	if currentRatio > targetRatio {
		// Current is wider, reduce width
		newW = int(float64(bounds.H) * targetRatio)
		newH = bounds.H
	} else {
		// Current is taller, reduce height
		newW = bounds.W
		newH = int(float64(bounds.W) / targetRatio)
	}

	// Ensure we don't exceed original bounds
	if newW > bounds.W {
		newW = bounds.W
		newH = int(float64(newW) / targetRatio)
	}
	if newH > bounds.H {
		newH = bounds.H
		newW = int(float64(newH) * targetRatio)
	}

	newX := centerX - newW/2
	newY := centerY - newH/2

	return CropBounds{
		X:     newX,
		Y:     newY,
		W:     newW,
		H:     newH,
		Valid: true,
	}
}

// SuggestCropSize returns the suggested crop dimensions based on detected
// watermarks. Returns (0, 0) if cropping is not applicable.
func SuggestCropSize(imgW, imgH int, regions []CropRegion) (int, int) {
	bounds := ComputeCropBounds(imgW, imgH, regions)
	if !bounds.Valid {
		return 0, 0
	}
	return bounds.W, bounds.H
}
