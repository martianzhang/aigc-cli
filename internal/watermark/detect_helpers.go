package watermark

import (
	"image"
	"math"
)

// scoreCandidate computes three-stage NCC for a single candidate position.
// size is the square size used for detection (min of w/h for rectangular alpha maps).
func scoreCandidate(gray, grad []float64, imgW, imgH int,
	alpha, alphaGrad []float64, cx, cy, size int) *candidate {

	if cx < 0 || cy < 0 || cx+size > imgW || cy+size > imgH {
		return nil
	}
	if size < 4 {
		return nil
	}

	// Extract region from grayscale and gradient
	// Use the square size for detection (alpha and alphaGrad are also size×size)
	grayRegion := make([]float64, size*size)
	gradRegion := make([]float64, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			gi := (cy+y)*imgW + cx + x
			li := y*size + x
			grayRegion[li] = gray[gi]
			gradRegion[li] = grad[gi]
		}
	}

	// Spatial NCC: grayscale vs alpha map
	spatial := ncc(grayRegion, alpha)

	// Gradient NCC: edges vs alpha map edges
	gradientScore := ncc(gradRegion, alphaGrad)

	// Variance analysis: watermark region should have lower variance
	varianceScore := 0.5 // neutral default
	refY := cy - size
	if refY >= 0 {
		refH := minInt(size, cy-refY)
		if refH > 8 {
			wmVar := regionVariance(gray, imgW, cx, cy, size, size)
			refVar := regionVariance(gray, imgW, cx, refY, size, refH)
			if refVar > 1e-10 {
				v := 1 - wmVar/refVar
				if v > 0 {
					varianceScore = v
				}
				if varianceScore > 1 {
					varianceScore = 1
				}
			}
		}
	}

	// Three-stage weighted fusion
	if spatial < 0 {
		spatial = 0
	}
	if gradientScore < 0 {
		gradientScore = 0
	}
	confidence := spatial*0.5 + gradientScore*0.3 + varianceScore*0.2

	return &candidate{
		x: cx, y: cy, size: size,
		spatial:    spatial,
		gradient:   gradientScore,
		variance:   varianceScore,
		confidence: confidence,
	}
}

// scoreCandidateRect scores a rectangular region with a rectangular alpha map.
// Used for text watermarks (Doubao, Jimeng) where the alpha map is wider than tall.
// The alpha and alphaGrad should already be resized to aw×ah.
func scoreCandidateRect(gray, grad []float64, imgW, imgH int,
	alpha, alphaGrad []float64, cx, cy, aw, ah int) *candidate {

	if cx < 0 || cy < 0 || cx+aw > imgW || cy+ah > imgH {
		return nil
	}
	if aw < 4 || ah < 4 {
		return nil
	}

	// Extract rectangular region from grayscale and gradient
	grayRegion := make([]float64, aw*ah)
	gradRegion := make([]float64, aw*ah)
	for y := 0; y < ah; y++ {
		for x := 0; x < aw; x++ {
			gi := (cy+y)*imgW + cx + x
			li := y*aw + x
			grayRegion[li] = gray[gi]
			gradRegion[li] = grad[gi]
		}
	}

	// Spatial NCC: grayscale vs alpha map (both are aw×ah)
	spatial := ncc(grayRegion, alpha)

	// Gradient NCC: edges vs alpha map edges
	gradientScore := ncc(gradRegion, alphaGrad)

	// Variance analysis
	varianceScore := 0.5
	refY := cy - ah
	if refY >= 0 {
		refH := minInt(ah, cy-refY)
		if refH > 8 {
			wmVar := regionVariance(gray, imgW, cx, cy, aw, ah)
			refVar := regionVariance(gray, imgW, cx, refY, aw, refH)
			if refVar > 1e-10 {
				v := 1 - wmVar/refVar
				if v > 0 {
					varianceScore = v
				}
				if varianceScore > 1 {
					varianceScore = 1
				}
			}
		}
	}

	if spatial < 0 {
		spatial = 0
	}
	if gradientScore < 0 {
		gradientScore = 0
	}
	confidence := spatial*0.5 + gradientScore*0.3 + varianceScore*0.2

	// Use min dimension as size for compatibility
	sz := aw
	if ah < sz {
		sz = ah
	}
	return &candidate{
		x: cx, y: cy, size: sz, w: aw, h: ah,
		spatial:    spatial,
		gradient:   gradientScore,
		variance:   varianceScore,
		confidence: confidence,
	}
}

// toGrayscale converts an image to normalized [0,1] float64 grayscale.
func toGrayscale(img image.Image, w, h int) []float64 {
	gray := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			lum := 0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(b>>8)
			gray[y*w+x] = lum / 255.0
		}
	}
	return gray
}

// sobelMagnitude computes 3×3 Sobel gradient magnitude.
func sobelMagnitude(data []float64, w, h int) []float64 {
	grad := make([]float64, w*h)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			i := y*w + x
			gx := -data[i-w-1] - 2*data[i-1] - data[i+w-1] +
				data[i-w+1] + 2*data[i+1] + data[i+w+1]
			gy := -data[i-w-1] - 2*data[i-w] - data[i-w+1] +
				data[i+w-1] + 2*data[i+w] + data[i+w+1]
			grad[i] = math.Sqrt(gx*gx + gy*gy)
		}
	}
	return grad
}

// ncc computes normalized cross-correlation between two float64 slices.
func ncc(a, b []float64) float64 {
	n := len(a)
	if n == 0 || n != len(b) {
		return 0
	}
	var sumA, sumB float64
	for i := 0; i < n; i++ {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)
	var num, denA, denB float64
	for i := 0; i < n; i++ {
		da := a[i] - meanA
		db := b[i] - meanB
		num += da * db
		denA += da * da
		denB += db * db
	}
	den := math.Sqrt(denA * denB)
	if den < 1e-10 {
		return 0
	}
	return num / den
}

// regionVariance computes the variance of a rectangular region in grayscale.
func regionVariance(gray []float64, stride int, x, y, w, h int) float64 {
	var sum, sumSq, n float64
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			v := gray[(y+dy)*stride+x+dx]
			sum += v
			sumSq += v * v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	mean := sum / n
	return sumSq/n - mean*mean
}

// resizeAlpha bilinearly resizes an alpha map from (sw, sh) to (dw, dh).
func resizeAlpha(src []float64, sw, sh, dw, dh int) []float64 {
	if dw == sw && dh == sh {
		dst := make([]float64, dw*dh)
		copy(dst, src)
		return dst
	}
	dst := make([]float64, dw*dh)
	for dy := 0; dy < dh; dy++ {
		sy := float64(dy) * float64(sh-1) / float64(maxInt(dh-1, 1))
		iy0 := int(sy)
		iy1 := minInt(iy0+1, sh-1)
		fy := sy - float64(iy0)
		for dx := 0; dx < dw; dx++ {
			sx := float64(dx) * float64(sw-1) / float64(maxInt(dw-1, 1))
			ix0 := int(sx)
			ix1 := minInt(ix0+1, sw-1)
			fx := sx - float64(ix0)

			v00 := src[iy0*sw+ix0]
			v10 := src[iy0*sw+ix1]
			v01 := src[iy1*sw+ix0]
			v11 := src[iy1*sw+ix1]

			top := v00*(1-fx) + v10*fx
			bot := v01*(1-fx) + v11*fx
			dst[dy*dw+dx] = top*(1-fy) + bot*fy
		}
	}
	return dst
}

// ── Binary mask extraction + NCC alignment (text watermarks) ──────────

// TextMarkParams holds the per-mark tuning for binary mask extraction.
// Mirrors TextMarkConfig from the remove-ai-watermarks reference project.
type TextMarkParams struct {
	MaxSaturation  float64 // max RGB channel spread to count as "grayish"
	LogoMinLuma    float64 // minimum absolute brightness for watermark pixels
	TophatDelta    float64 // minimum brightness above local background
	MorphOpenSize  int     // morphological open kernel side
	AlignSearchMin float64 // minimum scale for NCC alignment search
	AlignSearchMax float64 // maximum scale for NCC alignment search
}

// DefaultDoubaoParams returns the reference project's Doubao tuning.
func DefaultDoubaoParams() TextMarkParams {
	return TextMarkParams{
		MaxSaturation:  55,
		LogoMinLuma:    150,
		TophatDelta:    12,
		MorphOpenSize:  5,
		AlignSearchMin: 0.60,
		AlignSearchMax: 1.40,
	}
}

// extractBinaryMask extracts a binary mask of watermark-like pixels from a
// region of the image. A pixel is marked if it is:
//   - Low-saturation (max RGB channel spread < MaxSaturation)
//   - Brighter than local background by > TophatDelta (white top-hat)
//   - Absolutely bright (luma > LogoMinLuma)
//
// The mask is cleaned with morphological close (5×5) then open (MorphOpenSize).
// This mirrors extract_mask() from the remove-ai-watermarks reference project.
//
// Returns a bw×bh float64 slice (1.0 = watermark pixel, 0.0 = background).
func extractBinaryMask(img image.Image, bx, by, bw, bh int, p TextMarkParams) []float64 {
	if bh < 16 || bw < 16 {
		return make([]float64, bw*bh)
	}

	// Extract RGB + luma for the region
	rCh := make([]float64, bw*bh)
	gCh := make([]float64, bw*bh)
	bCh := make([]float64, bw*bh)
	luma := make([]float64, bw*bh)
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			cr, cg, cb, _ := img.At(bx+x, by+y).RGBA()
			r8, g8, b8 := float64(cr>>8), float64(cg>>8), float64(cb>>8)
			i := y*bw + x
			rCh[i] = r8
			gCh[i] = g8
			bCh[i] = b8
			luma[i] = (0.2126*r8 + 0.7152*g8 + 0.0722*b8) / 255.0
		}
	}

	// Local background: Gaussian blur of luma (sigma ~ box height * 0.4)
	sigma := math.Max(4.0, float64(bh)*0.4)
	localBg := gaussianBlur(luma, bw, bh, sigma)

	// Build binary mask
	mask := make([]float64, bw*bh)
	for i := 0; i < bw*bh; i++ {
		l := luma[i] * 255.0
		bg := localBg[i] * 255.0
		tophat := l - bg

		mx := math.Max(rCh[i], math.Max(gCh[i], bCh[i]))
		mn := math.Min(rCh[i], math.Min(gCh[i], bCh[i]))
		sat := mx - mn

		if sat < p.MaxSaturation && tophat > p.TophatDelta && l > p.LogoMinLuma {
			mask[i] = 1.0
		}
	}

	// Morphological close (5×5) then open (MorphOpenSize)
	mask = dilateBinary(mask, bw, bh, 5)
	mask = erodeBinary(mask, bw, bh, 5)
	mask = erodeBinary(mask, bw, bh, p.MorphOpenSize)
	mask = dilateBinary(mask, bw, bh, p.MorphOpenSize)

	return mask
}

// dilateBinary applies a max filter (dilation) with a square kernel of side k.
func dilateBinary(data []float64, w, h, k int) []float64 {
	out := make([]float64, w*h)
	half := k / 2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			mx := 0.0
			for dy := -half; dy <= half; dy++ {
				for dx := -half; dx <= half; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if data[ny*w+nx] > mx {
						mx = data[ny*w+nx]
					}
				}
			}
			out[y*w+x] = mx
		}
	}
	return out
}

// erodeBinary applies a min filter (erosion) with a square kernel of side k.
// Out-of-bounds is treated as 0 (erodes edges).
func erodeBinary(data []float64, w, h, k int) []float64 {
	out := make([]float64, w*h)
	half := k / 2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			mn := 1.0
			for dy := -half; dy <= half; dy++ {
				for dx := -half; dx <= half; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						mn = 0.0 // out-of-bounds = 0 → erodes
						continue
					}
					if data[ny*w+nx] < mn {
						mn = data[ny*w+nx]
					}
				}
			}
			out[y*w+x] = mn
		}
	}
	return out
}

// alignByNCC finds the best watermark position by sliding the alpha silhouette
// over a binary mask at multiple scales. This mirrors _aligned_alpha_map()
// from the remove-ai-watermarks reference project, which uses cv2.matchTemplate
// with TM_CCOEFF_NORMED (zero-mean NCC).
//
// Returns: bestX, bestY (absolute position in original image), bestW, bestH
// (alpha dimensions at best scale), bestScore (NCC confidence).
func alignByNCC(mask []float64, maskW, maskH, maskX, maskY int,
	srcAlpha []float64, srcW, srcH, expectW int, p TextMarkParams,
) (bestX, bestY, bestW, bestH int, bestScore float64) {

	// Build alpha silhouette (binary: alpha > 0.15)
	silAlpha := make([]float64, srcW*srcH)
	for i := range srcAlpha {
		if srcAlpha[i] > 0.15 {
			silAlpha[i] = 1.0
		}
	}

	// Scale search: 11 steps from AlignSearchMin to AlignSearchMax
	nSteps := 11
	scales := make([]float64, nSteps)
	for i := 0; i < nSteps; i++ {
		scales[i] = p.AlignSearchMin + (p.AlignSearchMax-p.AlignSearchMin)*float64(i)/float64(nSteps-1)
	}

	bestScore = -1.0
	bestX, bestY = maskX, maskY
	bestW, bestH = expectW, int(float64(expectW)*float64(srcH)/float64(srcW))

	for _, scale := range scales {
		aw := int(math.Round(float64(expectW) * scale))
		ah := int(math.Round(float64(aw) * float64(srcH) / float64(srcW)))
		if aw < 8 || ah < 4 || aw >= maskW || ah >= maskH {
			continue
		}

		// Resize silhouette to this scale
		rsSil := resizeAlpha(silAlpha, srcW, srcH, aw, ah)

		// Precompute template stats
		var tSum, tSumSq float64
		n := float64(aw * ah)
		for i := range rsSil {
			tSum += rsSil[i]
			tSumSq += rsSil[i] * rsSil[i]
		}
		tMean := tSum / n
		tStdSq := tSumSq/n - tMean*tMean
		if tStdSq < 1e-10 {
			continue
		}
		tStd := math.Sqrt(tStdSq)

		// Coarse search (stride 4)
		coarseBX, coarseBY := 0, 0
		coarseBest := -1.0
		for my := 0; my+ah <= maskH; my += 4 {
			for mx := 0; mx+aw <= maskW; mx += 4 {
				score := nccRegionFast(mask, maskW, mx, my, aw, ah, rsSil, tMean, tStd)
				if score > coarseBest {
					coarseBest = score
					coarseBX = mx
					coarseBY = my
				}
			}
		}

		// Fine search (stride 1, ±4 around best)
		fineMinX := maxInt(0, coarseBX-4)
		fineMaxX := minInt(maskW-aw, coarseBX+4)
		fineMinY := maxInt(0, coarseBY-4)
		fineMaxY := minInt(maskH-ah, coarseBY+4)
		for my := fineMinY; my <= fineMaxY; my++ {
			for mx := fineMinX; mx <= fineMaxX; mx++ {
				score := nccRegionFast(mask, maskW, mx, my, aw, ah, rsSil, tMean, tStd)
				if score > bestScore {
					bestScore = score
					bestX = maskX + mx
					bestY = maskY + my
					bestW = aw
					bestH = ah
				}
			}
		}
	}

	if bestScore < 0 {
		return 0, 0, 0, 0, 0
	}
	return bestX, bestY, bestW, bestH, bestScore
}

// nccRegionFast computes zero-mean NCC between a region of the mask and a
// pre-resized template. Template mean and std are precomputed.
func nccRegionFast(mask []float64, maskW, mx, my, aw, ah int,
	template []float64, tMean, tStd float64) float64 {

	n := float64(aw * ah)
	var mSum, mSumSq, dot float64
	for y := 0; y < ah; y++ {
		mi := (my+y)*maskW + mx
		ti := y * aw
		for x := 0; x < aw; x++ {
			mv := mask[mi+x]
			tv := template[ti+x]
			mSum += mv
			mSumSq += mv * mv
			dot += mv * tv
		}
	}
	mMean := mSum / n
	mStdSq := mSumSq/n - mMean*mMean
	if mStdSq < 1e-10 {
		return 0
	}
	return (dot/n - mMean*tMean) / (math.Sqrt(mStdSq) * tStd)
}

// insertTop5 inserts a candidate into a sorted slice, keeping at most n.
func insertTop5(slice *[]candidate, c *candidate, n int) {
	*slice = append(*slice, *c)
	for i := len(*slice) - 1; i > 0; i-- {
		if (*slice)[i].confidence > (*slice)[i-1].confidence {
			(*slice)[i], (*slice)[i-1] = (*slice)[i-1], (*slice)[i]
		}
	}
	if len(*slice) > n {
		*slice = (*slice)[:n]
	}
}
