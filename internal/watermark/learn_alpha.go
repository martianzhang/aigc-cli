package watermark

import (
	"image"
	"image/color"
	"sort"
)
func computeAlphaWith(black, gray image.Image, w, h int,
	bgB, bgG *bgModel,
	noiseFloorB, noiseFloorG float64,
	gradient bool) *AlphaMap {

	data := make([]float64, w*h)
	for y := 0; y < h; y++ {
		var bgBR, bgBG, bgBB, bgGR, bgGG, bgGB float64
		if gradient {
			bgBR, bgBG, bgBB = bgB.bgAt(0, y)
			bgGR, bgGG, bgGB = bgG.bgAt(0, y)
		} else {
			bgBR, bgBG, bgBB = bgB.baseR, bgB.baseG, bgB.baseB
			bgGR, bgGG, bgGB = bgG.baseR, bgG.baseG, bgG.baseB
		}

		for x := 0; x < w; x++ {
			if gradient && x > 0 {
				bgBR += bgB.gxR
				bgBG += bgB.gxG
				bgBB += bgB.gxB
				bgGR += bgG.gxR
				bgGG += bgG.gxG
				bgGB += bgG.gxB
			}

			br, bgg, bb, _ := black.At(x, y).RGBA()
			gr, gg, gb, _ := gray.At(x, y).RGBA()

			bR := float64(br >> 8)
			bG := float64(bgg >> 8)
			bB := float64(bb >> 8)
			gR := float64(gr >> 8)
			gG := float64(gg >> 8)
			gB := float64(gb >> 8)

			denBX := 255.0 - bgBR
			denGX := 255.0 - bgGR
			if denBX < 1 {
				denBX = 1
			}
			if denGX < 1 {
				denGX = 1
			}

			bDiffR := maxFloat(0, bR-bgBR-noiseFloorB)
			gDiffR := maxFloat(0, gR-bgGR-noiseFloorG)
			bDiffG := maxFloat(0, bG-bgBG-noiseFloorB)
			gDiffG := maxFloat(0, gG-bgGG-noiseFloorG)
			bDiffB := maxFloat(0, bB-bgBB-noiseFloorB)
			gDiffB := maxFloat(0, gB-bgGB-noiseFloorG)

			alpha := (bDiffR/denBX + gDiffR/denGX +
				bDiffG/denBX + gDiffG/denGX +
				bDiffB/denBX + gDiffB/denGX) / 6.0
			if alpha > 1 {
				alpha = 1
			}
			data[y*w+x] = alpha
		}
	}

	// Spatial smoothing
	smoothed := gaussianBlur(data, w, h, 1.0)

	// Noise gate — fixed at 0.001 (only true numerical noise).
	// The per-pixel noiseFloor (= 3*background_std) already handles
	// all per-seed noise suppression in the alpha computation.
	//
	// A dynamic gate (e.g. max(0.005, min(0.03, stdSum/200))) was
	// found to conflict with the TrimAlphaMap threshold for low-alpha
	// watermarks: the gate cut before the trim, removing faint sparkle
	// edges that the trim should have preserved.  Since the noise floor
	// already does the suppression, this gate only needs to eliminate
	// floating-point epsilon noise.
	for i, v := range smoothed {
		if v < 0.001 {
			smoothed[i] = 0
		}
	}

	return &AlphaMap{Width: w, Height: h, Data: smoothed}
}

// alphaConcentration measures how tightly the alpha mass is concentrated.
// Higher score = more focused watermark (less noise spread).
// score = total_alpha_mass / (non_zero_pixels + 1)
func alphaConcentration(am *AlphaMap) float64 {
	var mass, nz float64
	for _, v := range am.Data {
		if v > 0.001 {
			mass += v
			nz++
		}
	}
	if nz < 1 {
		return 0
	}
	// Also factor in area ratio: prefer smaller bounding boxes
	areaRatio := float64(am.Width*am.Height) / float64(maxInt(am.Width, am.Height)*maxInt(am.Width, am.Height))
	return (mass / nz) * areaRatio
}

// medianFilter3x3 applies a 3×3 median filter to an image's RGB channels.
// This removes isolated outlier pixels (e.g. hot pixels in noisy seeds)
// while preserving edges and watermark strokes — unlike a Gaussian blur
// which would smear outliers into neighbors.
//
// Used to denoise seed images before alpha computation.  High-noise seeds
// (std > 8) produce inflated noiseFloor thresholds that suppress weak
// watermark signal.  Median filtering the seed first drops the effective
// noise std, which lowers the noiseFloor and recovers faint alpha.
func medianFilter3x3(img image.Image) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(b)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var rVals, gVals, bVals [9]float64
			count := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < b.Min.X || nx >= b.Max.X || ny < b.Min.Y || ny >= b.Max.Y {
						continue
					}
					r, g, bv, _ := img.At(nx, ny).RGBA()
					rVals[count] = float64(r >> 8)
					gVals[count] = float64(g >> 8)
					bVals[count] = float64(bv >> 8)
					count++
				}
			}
			if count == 0 {
				r, g, bv, _ := img.At(x, y).RGBA()
				dst.Set(x, y, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bv >> 8), A: 255})
				continue
			}
			rSorted := rVals[:count]
			gSorted := gVals[:count]
			bSorted := bVals[:count]
			sort.Float64s(rSorted)
			sort.Float64s(gSorted)
			sort.Float64s(bSorted)
			mid := count / 2
			dst.Set(x, y, color.RGBA{
				R: uint8(rSorted[mid]),
				G: uint8(gSorted[mid]),
				B: uint8(bSorted[mid]),
				A: 255,
			})
		}
	}
	return dst
}

// SolveAlphaMap solves for the alpha map from black and gray seed images
// using the two-capture method.  It auto-adapts between two strategies:
//
//  1. Constant model (default) — assumes uniform background; works well for
//     clean seeds where bg is close to expected (black≈0, gray≈128).
//     Better for high-contrast text watermarks (baidu, doubao).
//
//  2. Gradient model — fits a linear gradient from the image edges to
//     compensate for brightness gradients in the seed images.
//     Better for seeds with non-uniform backgrounds (zhipu).
//
// Both models are computed and the one with higher alpha concentration
// (more watermark signal per pixel) is selected automatically.
//
// Before computing alpha, seed images are pre-denoised with a 3×3 median
// filter when their noise level is high (edge noise > 5).  This removes
// isolated outlier pixels that inflate the noise floor and suppress weak
// watermark signal — a problem observed on seeds from platforms with noisy
// generation pipelines.  The median filter preserves edges and strokes,
// unlike a Gaussian blur which would smear outliers into neighboring pixels.
func SolveAlphaMap(black, gray image.Image) *AlphaMap {
	b := black.Bounds()
	g := gray.Bounds()
	w, h := b.Dx(), b.Dy()
	if g.Dx() < w {
		w = g.Dx()
	}
	if g.Dy() < h {
		h = g.Dy()
	}

	// Pre-denoise: if seed noise is high, apply 3×3 median filter to remove
	// isolated outlier pixels before alpha computation.  The noise floor is
	// 3× background std, so outliers with std > 5 inflate the floor to > 15
	// and suppress weak watermark alpha.  Median filtering preserves edges
	// (watermark strokes) while removing salt-and-pepper noise.
	bgPreB, bgPreG := calibrateBackground(black, gray, w, h)
	if bgPreB.lumStd > 5 || bgPreG.lumStd > 5 {
		black = medianFilter3x3(black)
		gray = medianFilter3x3(gray)
	}

	// Shared noise parameters (computed from corner std, not gradient-dependent)
	bgConstB, bgConstG := calibrateBackground(black, gray, w, h)
	noiseFloorB := maxFloat(3.0, 3.0*bgConstB.lumStd)
	noiseFloorG := maxFloat(3.0, 3.0*bgConstG.lumStd)

	// Strategy 1: constant bg model
	constB := &bgModel{baseR: bgConstB.baseR, baseG: bgConstB.baseG, baseB: bgConstB.baseB}
	constG := &bgModel{baseR: bgConstG.baseR, baseG: bgConstG.baseG, baseB: bgConstG.baseB}
	resultConst := computeAlphaWith(black, gray, w, h, constB, constG, noiseFloorB, noiseFloorG, false)

	// Strategy 2: gradient bg model
	gradB := &bgModel{
		baseR: bgConstB.baseR, baseG: bgConstB.baseG, baseB: bgConstB.baseB,
		gxR: bgConstB.gxR, gxG: bgConstB.gxG, gxB: bgConstB.gxB,
		gyR: bgConstB.gyR, gyG: bgConstB.gyG, gyB: bgConstB.gyB,
	}
	gradG := &bgModel{
		baseR: bgConstG.baseR, baseG: bgConstG.baseG, baseB: bgConstG.baseB,
		gxR: bgConstG.gxR, gxG: bgConstG.gxG, gxB: bgConstG.gxB,
		gyR: bgConstG.gyR, gyG: bgConstG.gyG, gyB: bgConstG.gyB,
	}
	resultGrad := computeAlphaWith(black, gray, w, h, gradB, gradG, noiseFloorB, noiseFloorG, true)

	// Pick the model with higher concentration (more signal per pixel)
	scoreConst := alphaConcentration(resultConst)
	scoreGrad := alphaConcentration(resultGrad)

	if scoreGrad > scoreConst {
		return resultGrad
	}
	return resultConst
}

// combineSeeds merges multiple black (or gray) seed images into one,
// after calibrating each seed's background to a common baseline.
// This extracts the common watermark signal while canceling per-image noise.
//
// Uses per-pixel MEDIAN aggregation instead of mean: median is robust to
// outlier pixels (e.g. hot pixels with value 236 on a black seed) that
// would skew the mean and inflate the noise floor.  For N seeds, a single
// outlier pixel can shift the mean by outlier/N, but the median is
// completely unaffected as long as outliers are < 50% of the samples.
func combineSeeds(seeds []image.Image, bgBase float64) *image.RGBA {
	if len(seeds) == 0 {
		return nil
	}
	b := seeds[0].Bounds()
	w, h := b.Dx(), b.Dy()

	// Collect per-pixel calibrated values from all valid seeds
	type pxChan struct {
		r, g, b []float64
	}
	accum := make([]pxChan, w*h)
	var n int

	for _, img := range seeds {
		ib := img.Bounds()
		if ib.Dx() != w || ib.Dy() != h {
			continue
		}
		// Estimate this seed's average background from the top-left corner
		var bgR, bgG, bgB float64
		var bgN float64
		for y := 0; y < 40 && y < h; y++ {
			for x := 0; x < 40 && x < w; x++ {
				r, g, bv, _ := img.At(x, y).RGBA()
				bgR += float64(r >> 8)
				bgG += float64(g >> 8)
				bgB += float64(bv >> 8)
				bgN++
			}
		}
		if bgN == 0 {
			continue
		}
		bgR /= bgN
		bgG /= bgN
		bgB /= bgN

		// Shift this seed's pixels so its background matches bgBase
		// watermark = pixel - bg  →  calibrated = bgBase + watermark
		shiftR := bgBase - bgR
		shiftG := bgBase - bgG
		shiftB := bgBase - bgB

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, bv, _ := img.At(x, y).RGBA()
				idx := y*w + x
				accum[idx].r = append(accum[idx].r, float64(r>>8)+shiftR)
				accum[idx].g = append(accum[idx].g, float64(g>>8)+shiftG)
				accum[idx].b = append(accum[idx].b, float64(bv>>8)+shiftB)
			}
		}
		n++
	}

	if n == 0 {
		return nil
	}

	// Build the median-aggregated image
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			rSorted := accum[idx].r
			gSorted := accum[idx].g
			bSorted := accum[idx].b
			sort.Float64s(rSorted)
			sort.Float64s(gSorted)
			sort.Float64s(bSorted)
			mid := len(rSorted) / 2
			r := clampByte(rSorted[mid])
			g := clampByte(gSorted[mid])
			bv := clampByte(bSorted[mid])
			dst.Set(x, y, color.RGBA{R: r, G: g, B: bv, A: 255})
		}
	}
	return dst
}

// SolveAlphaMapMulti averages seed IMAGES at the pixel level (not alpha maps).
// All black seeds are background-calibrated to ~0 and averaged together;
// all gray seeds are background-calibrated to ~128 and averaged together.
// The two composites are then solved via the standard two-capture method.
// This extracts the common watermark signal while canceling per-image noise.
// Pairs are NOT required — every black and every gray contributes independently.
func SolveAlphaMapMulti(blacks, grays []image.Image) *AlphaMap {
	if len(blacks) == 0 || len(grays) == 0 {
		return nil
	}
	compositeBlack := combineSeeds(blacks, 0)
	compositeGray := combineSeeds(grays, 128)
	if compositeBlack == nil || compositeGray == nil {
		return nil
	}
	return SolveAlphaMap(compositeBlack, compositeGray)
}

// maxFloat returns the larger of two float64 values.
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// TrimAlphaMap trims transparent edges from the alpha map.
// Pixels below threshold are considered transparent and guide the
// bounding box; a 2-pixel padding is added around the result.
func TrimAlphaMap(am *AlphaMap, threshold float64) (*AlphaMap, int, int, int, int) {
	if am.Width == 0 || am.Height == 0 {
		return am, 0, 0, 0, 0
	}
	minX, minY := am.Width, am.Height
	maxX, maxY := 0, 0
	for y := 0; y < am.Height; y++ {
		for x := 0; x < am.Width; x++ {
			if am.Data[y*am.Width+x] > threshold {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if minX > maxX || minY > maxY {
		return am, 0, 0, am.Width, am.Height
	}
	pad := 2
	minX = maxInt(0, minX-pad)
	minY = maxInt(0, minY-pad)
	maxX = minInt(am.Width-1, maxX+pad)
	maxY = minInt(am.Height-1, maxY+pad)
	cw := maxX - minX + 1
	ch := maxY - minY + 1
	data := make([]float64, cw*ch)
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			data[y*cw+x] = am.Data[(minY+y)*am.Width+(minX+x)]
		}
	}
	return &AlphaMap{Width: cw, Height: ch, Data: data}, minX, minY, cw, ch
}

// TrimAlphaMapMass trims edges by keeping the smallest region that contains
// at least `fraction` of the total alpha mass.  This adapts automatically
// to both high-alpha text marks and low-alpha sparkle patterns.
// alphaMass is sum(alpha) over the full map; pass 0 to compute it here.
func TrimAlphaMapMass(am *AlphaMap, fraction float64) (*AlphaMap, int, int, int, int) {
	if am.Width == 0 || am.Height == 0 || fraction >= 1 {
		return TrimAlphaMap(am, 0)
	}

	// Compute total alpha mass
	totalMass := 0.0
	for i := range am.Data {
		totalMass += am.Data[i]
	}
	if totalMass < 1e-10 {
		return TrimAlphaMap(am, 0)
	}

	// Compute row and column marginals
	rowMass := make([]float64, am.Height)
	colMass := make([]float64, am.Width)
	for y := 0; y < am.Height; y++ {
		for x := 0; x < am.Width; x++ {
			v := am.Data[y*am.Width+x]
			rowMass[y] += v
			colMass[x] += v
		}
	}

	// Trim from top until cumulative exceeds waste margin
	trimFraction := (1 - fraction) / 2
	waste := totalMass * trimFraction

	// Top
	var cum float64
	top := 0
	for top < am.Height && cum+rowMass[top] <= waste {
		cum += rowMass[top]
		top++
	}

	// Bottom
	cum = 0
	bot := am.Height - 1
	for bot >= 0 && cum+rowMass[bot] <= waste {
		cum += rowMass[bot]
		bot--
	}

	// Left
	cum = 0
	left := 0
	for left < am.Width && cum+colMass[left] <= waste {
		cum += colMass[left]
		left++
	}

	// Right
	cum = 0
	right := am.Width - 1
	for right >= 0 && cum+colMass[right] <= waste {
		cum += colMass[right]
		right--
	}

	if left > right || top > bot {
		return TrimAlphaMap(am, 0)
	}

	pad := 2
	left = maxInt(0, left-pad)
	top = maxInt(0, top-pad)
	right = minInt(am.Width-1, right+pad)
	bot = minInt(am.Height-1, bot+pad)

	cw := right - left + 1
	ch := bot - top + 1
	data := make([]float64, cw*ch)
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			data[y*cw+x] = am.Data[(top+y)*am.Width+(left+x)]
		}
	}
	return &AlphaMap{Width: cw, Height: ch, Data: data}, left, top, cw, ch
}

// LearnResult holds the result of learning a watermark from seed images.
