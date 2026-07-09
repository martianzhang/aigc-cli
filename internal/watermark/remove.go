package watermark

import (
	"image"
	"image/draw"
	"math"
)

// defaultAlphaGains lists alpha multipliers tried during removal,
// from most conservative to most aggressive.
var defaultAlphaGains = []float64{0.6, 0.8, 1.0, 1.15, 1.3}

// removeWatermark removes a detected watermark using reverse alpha blending.
// It tries multiple alpha gains and sub-pixel refinements.
func removeWatermark(img image.Image, det *candidate, cfg Config) *image.RGBA {
	if det == nil || det.size < 4 {
		return cloneToRGBA(img)
	}
	if cfg.RemoveStrategy == RemoveSkip {
		return cloneToRGBA(img)
	}

	srcAlpha := cfg.AlphaMap.Data
	srcW, srcH := cfg.AlphaMap.Width, cfg.AlphaMap.Height
	logo := cfg.LogoColor

	// For PositionResolver configs (text watermarks), use the detected rectangle dimensions.
	// For square alpha maps, det.w and det.h are 0, so fall back to det.size.
	dw, dh := det.w, det.h
	if dw <= 0 || dh <= 0 {
		dw, dh = det.size, det.size
	}
	size := dh           // use height for square sub-pixel logic; width may differ
	isTextWM := dw != dh // rectangular alpha = text watermark

	if cfg.RemoveStrategy == RemoveInpaint {
		// Badge watermarks have an opaque/semi-opaque dark background.
		// inpaintBadgeMedian replaces the entire badge area (alpha > 0.10)
		// using per-column median sampling from above the badge, preserving
		// background gradients without blurring.
		if cfg.Name == "doubao-snap" || cfg.Name == "baidu" {
			return inpaintBadgeMedian(cloneToRGBA(img), srcAlpha, srcW, srcH, det.x, det.y, dw, dh, 0.10)
		}
		baseAlpha := resizeAlpha(srcAlpha, srcW, srcH, dw, dh)
		return inpaintResidual(cloneToRGBA(img), baseAlpha, det.x, det.y, dw, dh, 0.08, 2, 3)
	}

	type trialResult struct {
		dst      *image.RGBA
		alpha    []float64
		residual float64
		gain     float64
	}

	// Multi-pass removal: apply repeatedly until residual is low.
	current := cloneToRGBA(img)
	baseResidual := math.MaxFloat64

	// Light background guard: on bright backgrounds (e.g., white images),
	// the watermark is nearly invisible (tophat ≈ 4*alpha ≈ 2.6 levels).
	// Reverse alpha amplifies noise by 1/(1-alpha) ≈ 3x, creating more
	// artifacts than it fixes. Skip reverse alpha entirely and use inpaint.
	if isTextWM {
		bgLuma := meanBackgroundLuma(img, det.x, det.y, dw, dh)
		if bgLuma > 200 {
			baseAlpha := resizeAlpha(srcAlpha, srcW, srcH, dw, dh)
			return inpaintResidual(current, baseAlpha, det.x, det.y, dw, dh, 0.05, 7, 3)
		}
	}

	for pass := 0; pass < 4; pass++ {
		baseAlpha := resizeAlpha(srcAlpha, srcW, srcH, dw, dh)
		best := trialResult{residual: math.MaxFloat64}

		// Phase 1: try multiple alpha gains
		for _, gain := range defaultAlphaGains {
			dst := applyReverseAlphaRect(current, baseAlpha, det.x, det.y, dw, dh, logo, gain)
			residual := computeResidualRect(dst, baseAlpha, det.x, det.y, dw, dh)
			if residual < best.residual {
				best = trialResult{dst, baseAlpha, residual, gain}
			}
		}
		if best.dst == nil {
			break
		}

		// Phase 2: sub-pixel refinement
		if dw == dh {
			for _, dx := range []float64{-0.5, -0.25, 0.25, 0.5} {
				for _, dy := range []float64{-0.5, -0.25, 0.25, 0.5} {
					for _, sc := range []float64{0.98, 1.0, 1.02} {
						warped := warpAlphaMap(srcAlpha, srcW, srcH, size, dx, dy, sc)
						dst := applyReverseAlpha(current, warped, det.x, det.y, size, logo, best.gain)
						residual := computeResidual(dst, warped, det.x, det.y, size)
						if residual < best.residual {
							best = trialResult{dst, warped, residual, best.gain}
						}
					}
				}
			}
		} else {
			// Position refinement for rectangular alpha maps (text watermarks).
			// Search ±3px to compensate for alpha map position mismatches
			// across different Doubao model versions.
			for _, dxi := range []int{-3, -2, -1, 0, 1, 2, 3} {
				for _, dyi := range []int{-3, -2, -1, 0, 1, 2, 3} {
					if dxi == 0 && dyi == 0 {
						continue
					}
					dx, dy := det.x+dxi, det.y+dyi
					if dx < 0 || dy < 0 || dx+dw > img.Bounds().Dx() || dy+dh > img.Bounds().Dy() {
						continue
					}
					dst2 := applyReverseAlphaRect(current, best.alpha, dx, dy, dw, dh, logo, best.gain)
					residual := computeResidualRect(dst2, best.alpha, dx, dy, dw, dh)
					if residual < best.residual {
						best = trialResult{dst2, best.alpha, residual, best.gain}
					}
				}
			}
		}

		// Phase 3: edge cleanup
		if dw == dh {
			best.dst = blendEdgeResidual(best.dst, best.alpha, det.x, det.y, size)
			// Thin inpaint over the very edge of the sparkle footprint to
			// smooth any residual boundary artifacts (JPEG compression noise,
			// sub-pixel misalignment). Only affects pixels with very low alpha
			// (the anti-aliased edge), preserving the reverse-alpha recovery
			// at the sparkle center.
			best.dst = inpaintResidual(best.dst, best.alpha, det.x, det.y, size, size, 0.01, 3, 3)
		} else {
			// Text watermark residual cleanup: dilate alpha mask + inpaint,
			// matching the reference project's approach (cv2.inpaint with
			// residual_alpha_floor=0.05, dilate=5, radius=2).
			best.dst = inpaintResidual(best.dst, best.alpha, det.x, det.y, dw, dh, 0.03, 7, 3)
		}

		// Stop if no improvement
		if best.residual >= baseResidual {
			return current
		}
		baseResidual = best.residual

		// Over-subtraction guard for text watermarks: if the recovered glyph area
		// is more than 25 gray levels below the surrounding background ring, fall
		// back to inpainting from the original (avoids dark pits).
		if isTextWM && pass == 0 && best.residual < baseResidual {
			margin := cfg.OversubtractMargin
			if margin <= 0 {
				margin = 25.0
			}
			bodyDark := checkOversubtraction(img, best.alpha, det.x, det.y, dw, dh, margin)
			if bodyDark {
				return inpaintResidual(cloneToRGBA(img), best.alpha, det.x, det.y, dw, dh, 0.05, 9, 4)
			}
		}

		// Stop early if residual is low enough
		if best.residual <= 0.25 {
			return best.dst
		}

		current = best.dst
	}

	return current
}

// blendEdgeResidual applies inverse-distance weighted neighborhood blending
// to alpha edge pixels, removing the last faint residual of the watermark.
// This mirrors blendPreviewResidualEdge from the reference JS project.
func blendEdgeResidual(src *image.RGBA, alpha []float64, ax, ay, size int) *image.RGBA {
	b := src.Bounds()
	imgW := b.Dx()
	imgH := b.Dy()

	// Build alpha gradient mask (Sobel + normalize + dilate + blur)
	gradMask := alphaGradientMask(alpha, size, size)

	radius := 3
	minAlpha := 0.02
	maxAlpha := 0.55
	outsideAlphaMax := 0.08
	strength := 0.7

	dst := cloneToRGBA(src)
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			localIdx := row*size + col
			a := alpha[localIdx]
			if a < minAlpha || a > maxAlpha {
				continue
			}

			var sumR, sumG, sumB, sumW float64
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := ax+col+dx, ay+row+dy
					if nx < 0 || ny < 0 || nx >= imgW || ny >= imgH {
						continue
					}
					// Check if neighbor is outside the alpha edge
					ly, lx := row+dy, col+dx
					var na float64
					if ly >= 0 && ly < size && lx >= 0 && lx < size {
						na = alpha[ly*size+lx]
					}
					if na > outsideAlphaMax {
						continue
					}
					dist := float64(dx*dx + dy*dy)
					if dist < 0.1 {
						dist = 0.1
					}
					w := 1.0 / dist
					nOff := src.PixOffset(nx, ny)
					sumR += float64(src.Pix[nOff+0]) * w
					sumG += float64(src.Pix[nOff+1]) * w
					sumB += float64(src.Pix[nOff+2]) * w
					sumW += w
				}
			}
			if sumW <= 0 {
				continue
			}

			edgeW := gradMask[localIdx]
			if edgeW < 0.35 {
				edgeW = 0.35
			}
			maxAlphaSafe := maxAlpha
			if maxAlphaSafe < 0.01 {
				maxAlphaSafe = 0.01
			}
			blend := strength * a / maxAlphaSafe * edgeW
			if blend < 0 {
				blend = 0
			}
			if blend > 1 {
				blend = 1
			}

			off := dst.PixOffset(ax+col, ay+row)
			avgR := sumR / sumW
			avgG := sumG / sumW
			avgB := sumB / sumW
			dst.Pix[off+0] = uint8(float64(src.Pix[off+0])*(1-blend) + avgR*blend + 0.5)
			dst.Pix[off+1] = uint8(float64(src.Pix[off+1])*(1-blend) + avgG*blend + 0.5)
			dst.Pix[off+2] = uint8(float64(src.Pix[off+2])*(1-blend) + avgB*blend + 0.5)
		}
	}
	return dst
}

// alphaGradientMask computes the Sobel gradient of the alpha map,
// normalizes it, dilates, and blurs — same as createAlphaGradientMask.
func alphaGradientMask(alpha []float64, w, h int) []float64 {
	grad := make([]float64, w*h)
	minG, maxG := math.MaxFloat64, 0.0
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			i := y*w + x
			gx := -alpha[i-w-1] - 2*alpha[i-1] - alpha[i+w-1] +
				alpha[i-w+1] + 2*alpha[i+1] + alpha[i+w+1]
			gy := -alpha[i-w-1] - 2*alpha[i-w] - alpha[i-w+1] +
				alpha[i+w-1] + 2*alpha[i+w] + alpha[i+w+1]
			v := math.Sqrt(gx*gx + gy*gy)
			grad[i] = v
			if v < minG {
				minG = v
			}
			if v > maxG {
				maxG = v
			}
		}
	}

	// Normalize [0,1]
	norm := make([]float64, w*h)
	rangeG := maxG - minG
	if rangeG > 1e-10 {
		for i := range norm {
			norm[i] = (grad[i] - minG) / rangeG
		}
	}

	// Dilate: max filter radius 2
	dil := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			mx := 0.0
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					if dx*dx+dy*dy > 4 {
						continue
					}
					sx, sy := x+dx, y+dy
					if sx < 0 || sy < 0 || sx >= w || sy >= h {
						continue
					}
					if norm[sy*w+sx] > mx {
						mx = norm[sy*w+sx]
					}
				}
			}
			dil[y*w+x] = mx
		}
	}

	// Gaussian blur (sigma=2, separable approximation)
	blur := gaussianBlur(dil, w, h, 2)
	return blur
}

// gaussianBlur applies a separable Gaussian blur (sigma=2, radius=6).
func gaussianBlur(data []float64, w, h int, sigma float64) []float64 {
	radius := int(sigma * 3)
	if radius < 1 {
		radius = 1
	}
	kernel := make([]float64, radius*2+1)
	var sum float64
	for i := -radius; i <= radius; i++ {
		v := math.Exp(-float64(i*i) / (2 * sigma * sigma))
		kernel[i+radius] = v
		sum += v
	}
	for i := range kernel {
		kernel[i] /= sum
	}

	// Horizontal blur
	tmp := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var s float64
			for k := -radius; k <= radius; k++ {
				sx := x + k
				if sx < 0 {
					sx = 0
				}
				if sx >= w {
					sx = w - 1
				}
				s += data[y*w+sx] * kernel[k+radius]
			}
			tmp[y*w+x] = s
		}
	}

	// Vertical blur
	out := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var s float64
			for k := -radius; k <= radius; k++ {
				sy := y + k
				if sy < 0 {
					sy = 0
				}
				if sy >= h {
					sy = h - 1
				}
				s += tmp[sy*w+x] * kernel[k+radius]
			}
			out[y*w+x] = s
		}
	}
	return out
}

// warpAlphaMap translates and scales an alpha map with sub-pixel precision
// using bilinear interpolation.  srcAlpha is the original full-size alpha map.
func warpAlphaMap(srcAlpha []float64, srcW, srcH, dstSize int, dx, dy, scale float64) []float64 {
	if dx == 0 && dy == 0 && scale == 1 {
		// No warp needed — use the standard resize
		return resizeAlpha(srcAlpha, srcW, srcH, dstSize, dstSize)
	}

	dst := make([]float64, dstSize*dstSize)
	center := float64(dstSize-1) / 2

	sample := func(sx, sy float64) float64 {
		ix0 := int(sx)
		iy0 := int(sy)
		if ix0 < 0 || ix0 >= dstSize-1 || iy0 < 0 || iy0 >= dstSize-1 {
			return 0
		}
		ix1 := ix0 + 1
		iy1 := iy0 + 1
		// Map back to source alpha coordinates
		sx0 := float64(ix0) * float64(srcW-1) / float64(maxInt(dstSize-1, 1))
		sx1 := float64(ix1) * float64(srcW-1) / float64(maxInt(dstSize-1, 1))
		sy0 := float64(iy0) * float64(srcH-1) / float64(maxInt(dstSize-1, 1))
		sy1 := float64(iy1) * float64(srcH-1) / float64(maxInt(dstSize-1, 1))

		siy0 := clampIdx(int(sy0), srcH)
		siy1 := clampIdx(int(sy1), srcH)
		six0 := clampIdx(int(sx0), srcW)
		six1 := clampIdx(int(sx1), srcW)

		fsy := sy0 - float64(int(sy0))
		fsx := sx0 - float64(int(sx0))

		v00 := srcAlpha[siy0*srcW+six0]
		v10 := srcAlpha[siy0*srcW+six1]
		v01 := srcAlpha[siy1*srcW+six0]
		v11 := srcAlpha[siy1*srcW+six1]

		top := v00*(1-fsx) + v10*fsx
		bot := v01*(1-fsx) + v11*fsx
		return top*(1-fsy) + bot*fsy
	}

	for y := 0; y < dstSize; y++ {
		for x := 0; x < dstSize; x++ {
			sx := (float64(x)-center)/scale + center + dx
			sy := (float64(y)-center)/scale + center + dy
			dst[y*dstSize+x] = sample(sx, sy)
		}
	}
	return dst
}

func clampIdx(v, max int) int {
	if v < 0 {
		return 0
	}
	if v >= max {
		return max - 1
	}
	return v
}

// applyReverseAlphaRect is like applyReverseAlpha but for rectangular regions (dw × dh).
// original = (pixel - alpha*logo) / (1 - alpha), with alpha clamped to [0, 0.99] to
// bound noise amplification. A small baseline (0.0118) is subtracted before the gain
// test so near-zero alpha pixels are skipped.
func applyReverseAlphaRect(img image.Image, alpha []float64,
	x, y, dw, dh int, logo [3]float64, gain float64) *image.RGBA {

	b := img.Bounds()
	dst := cloneToRGBA(img)

	for dy := 0; dy < dh; dy++ {
		for dx := 0; dx < dw; dx++ {
			rawAlpha := alpha[dy*dw+dx]
			signalAlpha := (rawAlpha - 0.0118) * gain
			if signalAlpha < 0.002 {
				continue
			}
			a := rawAlpha * gain
			if a > 0.99 {
				a = 0.99
			}
			inv := 1.0 - a

			px, py := x+dx, y+dy
			if px < b.Min.X || px >= b.Max.X || py < b.Min.Y || py >= b.Max.Y {
				continue
			}

			off := dst.PixOffset(px, py)
			r := float64(dst.Pix[off+0]) + 0.5
			g := float64(dst.Pix[off+1]) + 0.5
			bl := float64(dst.Pix[off+2]) + 0.5

			nr := (r - a*logo[0]) / inv
			ng := (g - a*logo[1]) / inv
			nb := (bl - a*logo[2]) / inv

			dst.Pix[off+0] = clampByte(nr)
			dst.Pix[off+1] = clampByte(ng)
			dst.Pix[off+2] = clampByte(nb)
		}
	}
	return dst
}

// computeResidualRect is like computeResidual but for rectangular regions (dw × dh).
func computeResidualRect(dst *image.RGBA, alpha []float64, x, y, dw, dh int) float64 {
	b := dst.Bounds()
	if x+dw > b.Dx() || y+dh > b.Dy() {
		return 1
	}

	region := make([]float64, dw*dh)
	for dy := 0; dy < dh; dy++ {
		for dx := 0; dx < dw; dx++ {
			off := dst.PixOffset(x+dx, y+dy)
			r := float64(dst.Pix[off+0])
			g := float64(dst.Pix[off+1])
			bl := float64(dst.Pix[off+2])
			region[dy*dw+dx] = (0.2126*r + 0.7152*g + 0.0722*bl) / 255.0
		}
	}

	score := ncc(region, alpha)
	if score < 0 {
		score = 0
	}
	return score
}

// applyReverseAlpha performs reverse alpha blending on the watermark region.
//
//	original = (pixel - alpha * gain * logo) / (1 - alpha * gain)
//
// With over-subtraction guard: result must not be more than 30% darker
// than the original pixel.
func applyReverseAlpha(img image.Image, alpha []float64,
	x, y, size int, logo [3]float64, gain float64) *image.RGBA {

	b := img.Bounds()
	dst := cloneToRGBA(img)

	for dy := 0; dy < size; dy++ {
		for dx := 0; dx < size; dx++ {
			rawAlpha := alpha[dy*size+dx]
			// Noise floor subtraction (3/255): remove JPEG compression noise
			// from the alpha map, matching the reference project's approach.
			signalAlpha := (rawAlpha - 0.0118) * gain
			if signalAlpha < 0.002 {
				continue
			}
			a := rawAlpha * gain
			if a > 0.99 {
				a = 0.99
			}
			if a > 0.99 {
				a = 0.99
			}
			inv := 1.0 - a

			px, py := x+dx, y+dy
			if px < b.Min.X || px >= b.Max.X || py < b.Min.Y || py >= b.Max.Y {
				continue
			}

			off := dst.PixOffset(px, py)
			r := float64(dst.Pix[off+0]) + 0.5
			g := float64(dst.Pix[off+1]) + 0.5
			bl := float64(dst.Pix[off+2]) + 0.5

			// Reverse blend
			nr := (r - a*logo[0]) / inv
			ng := (g - a*logo[1]) / inv
			nb := (bl - a*logo[2]) / inv

			dst.Pix[off+0] = clampByte(nr)
			dst.Pix[off+1] = clampByte(ng)
			dst.Pix[off+2] = clampByte(nb)
		}
	}
	return dst
}

// computeResidual measures how much watermark remains after removal,
// by computing the spatial NCC between the cleaned region and the alpha
// map. Lower score = better removal.
func computeResidual(dst *image.RGBA, alpha []float64, x, y, size int) float64 {
	b := dst.Bounds()
	if x+size > b.Dx() || y+size > b.Dy() {
		return 1
	}

	// Extract grayscale from the cleaned region
	region := make([]float64, size*size)
	for dy := 0; dy < size; dy++ {
		for dx := 0; dx < size; dx++ {
			off := dst.PixOffset(x+dx, y+dy)
			r := float64(dst.Pix[off+0])
			g := float64(dst.Pix[off+1])
			bl := float64(dst.Pix[off+2])
			region[dy*size+dx] = (0.2126*r + 0.7152*g + 0.0722*bl) / 255.0
		}
	}

	score := ncc(region, alpha)
	if score < 0 {
		score = 0
	}
	return score
}

// checkOversubtraction predicts whether reverse-alpha would create a dark pit
// (glyph body >darkMargin gray levels below surrounding background ring).
// Mirrors the reference project's _reverse_alpha_oversubtracts gate.
// darkMargin is data-driven per producer (cfg.OversubtractMargin, default 25).
func checkOversubtraction(img image.Image, alpha []float64, ax, ay, aw, ah int, darkMargin float64) bool {
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if aw < 4 || ah < 4 {
		return false
	}
	// Check if alpha is strong enough to over-subtract
	var maxA float64
	for i := range alpha {
		if alpha[i] > maxA {
			maxA = alpha[i]
		}
	}
	if maxA < 0.2 {
		return false
	}
	// Build body mask (alpha > 0.15 = glyph strokes)
	bodyMask := make([]bool, aw*ah)
	hasBody := false
	for row := 0; row < ah; row++ {
		for col := 0; col < aw; col++ {
			if alpha[row*aw+col] > 0.15 {
				bodyMask[row*aw+col] = true
				hasBody = true
			}
		}
	}
	if !hasBody {
		return false
	}
	// Sample surrounding background ring (pad = 60% of height)
	pad := maxInt(4, int(float64(ah)*0.6))
	ry1 := maxInt(0, ay-pad)
	ry2 := minInt(ih, ay+ah+pad)
	rx1 := maxInt(0, ax-pad)
	rx2 := minInt(iw, ax+aw+pad)
	// Compute ring stats and body prediction
	var ringSum, ringCount float64
	var bodyDarkSum, bodyDarkCount float64
	logo := 255.0 // white logo

	for y := ry1; y < ry2; y++ {
		for x := rx1; x < rx2; x++ {
			inBody := x >= ax && x < ax+aw && y >= ay && y < ay+ah
			r, g, bl, _ := img.At(x, y).RGBA()
			lum := (0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(bl>>8))

			if inBody {
				col, row := x-ax, y-ay
				if bodyMask[row*aw+col] {
					a := alpha[row*aw+col]
					if a > 0.99 {
						a = 0.99
					}
					predicted := (lum - a*logo) / (1.0 - a)
					bodyDarkSum += predicted
					bodyDarkCount++
				}
			} else {
				ringSum += lum
				ringCount++
			}
		}
	}
	if ringCount < 10 || bodyDarkCount < 1 {
		return false
	}
	bg := ringSum / ringCount
	predictedCore := bodyDarkSum / bodyDarkCount
	return predictedCore < bg-darkMargin
}

// inpaintResidual removes residual watermark by progressive boundary-growing
// inpaint. Pixels where alpha > floor are dilated by `dilate` pixels to form
// a mask, then masked pixels are replaced layer by layer from the outside in:
//  1. Find masked pixels adjacent to non-masked (boundary layer)
//  2. Replace each with IDW average of non-masked neighbors (radius `radius`)
//  3. Mark as non-masked (becomes valid neighbor for next layer)
//  4. Repeat until no masked pixels remain
//
// This mirrors cv2.inpaint's Telea algorithm: a small-radius IDW is sufficient
// because each layer propagates the outer background inward.
func inpaintResidual(src *image.RGBA, alpha []float64, ax, ay, aw, ah int, floor float64, dilate, radius int) *image.RGBA {
	b := src.Bounds()
	imgW, imgH := b.Dx(), b.Dy()

	// Build flat binary mask: dilate the alpha footprint
	mask := make([]bool, imgW*imgH)
	for row := 0; row < ah; row++ {
		for col := 0; col < aw; col++ {
			if alpha[row*aw+col] > floor {
				py, px := ay+row, ax+col
				for dy := -dilate; dy <= dilate; dy++ {
					for dx := -dilate; dx <= dilate; dx++ {
						ny, nx := py+dy, px+dx
						if nx >= 0 && nx < imgW && ny >= 0 && ny < imgH {
							mask[ny*imgW+nx] = true
						}
					}
				}
			}
		}
	}

	dst := cloneToRGBA(src)

	// Progressive boundary-growing inpaint
	for {
		// Find boundary pixels: masked pixels with at least one non-masked neighbor
		type bp struct{ x, y int }
		var boundary []bp
		for y := 0; y < imgH; y++ {
			for x := 0; x < imgW; x++ {
				if !mask[y*imgW+x] {
					continue
				}
				// Check 4-connectivity for boundary detection
				for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
					nx, ny := x+d[0], y+d[1]
					if nx < 0 || ny < 0 || nx >= imgW || ny >= imgH {
						continue
					}
					if !mask[ny*imgW+nx] {
						boundary = append(boundary, bp{x, y})
						break
					}
				}
			}
		}

		if len(boundary) == 0 {
			break // all inpainted
		}

		// Inpaint boundary pixels using IDW from non-masked neighbors
		for _, p := range boundary {
			x, y := p.x, p.y
			var sumR, sumG, sumB, sumW float64
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= imgW || ny < 0 || ny >= imgH {
						continue
					}
					if mask[ny*imgW+nx] {
						continue // skip masked pixels
					}
					dist := float64(dx*dx + dy*dy)
					w := 1.0 / (dist + 0.1)
					off := dst.PixOffset(nx, ny)
					sumR += float64(dst.Pix[off+0]) * w
					sumG += float64(dst.Pix[off+1]) * w
					sumB += float64(dst.Pix[off+2]) * w
					sumW += w
				}
			}
			if sumW > 0 {
				off := dst.PixOffset(x, y)
				dst.Pix[off+0] = clampByte(sumR / sumW)
				dst.Pix[off+1] = clampByte(sumG / sumW)
				dst.Pix[off+2] = clampByte(sumB / sumW)
			}
		}

		// Unmask boundary pixels (they're now valid neighbors for next layer)
		for _, p := range boundary {
			mask[p.y*imgW+p.x] = false
		}
	}

	return dst
}

// cloneToRGBA converts any image to *image.RGBA.
func cloneToRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}

// meanBackgroundLuma samples the background ring around the watermark area
// and returns the mean luma (0-255). Used to detect light backgrounds where
// reverse alpha blending would amplify noise.
func meanBackgroundLuma(img image.Image, ax, ay, aw, ah int) float64 {
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	pad := maxInt(4, int(float64(ah)*0.6))
	ry1 := maxInt(0, ay-pad)
	ry2 := minInt(ih, ay+ah+pad)
	rx1 := maxInt(0, ax-pad)
	rx2 := minInt(iw, ax+aw+pad)

	var sum float64
	var count int
	for y := ry1; y < ry2; y++ {
		for x := rx1; x < rx2; x++ {
			inWM := x >= ax && x < ax+aw && y >= ay && y < ay+ah
			if inWM {
				continue
			}
			r, g, bl, _ := img.At(x, y).RGBA()
			sum += 0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(bl>>8)
			count++
		}
	}
	if count == 0 {
		return 128
	}
	return sum / float64(count)
}

// inpaintBadgeMedian replaces the watermark area with a natural-looking fill.
//
// Strategy: TEXTURE COPY from directly above the watermark strip.
//
//	For a corner-anchored watermark, the region directly above is the natural
//	continuation of the background. Copying that patch preserves noise,
//	subtle color variation and any gradient — avoiding the flat "color block"
//	artifact that a single median fill produces (fill std collapses to ~0
//	while the surrounding region has std ~15, and the eye picks that up
//	immediately).
//
// After the copy we apply per-channel luma correction (aligning the copied
// patch's mean to the mean of the 8 rows immediately above the strip) and
// cross-fade the top 3 rows into the natural image above, so the seam
// disappears.
//
// If the watermark is too close to the image top for a full texture copy,
// we fall back to per-column median fill.
//
// alphaFloor controls which pixels get replaced:
//   - 0.10 for opaque badges (doubao-snap): only the dark badge body
//   - -1.0 for embedded text watermarks: the entire rectangle, since the whole
//     region is contaminated by the alpha-blend and low-alpha edges also carry
//     watermark signal
func inpaintBadgeMedian(src *image.RGBA, srcAlpha []float64, srcW, srcH, ax, ay, aw, ah int, alphaFloor float64) *image.RGBA {
	b := src.Bounds()
	imgW := b.Dx()
	imgH := b.Dy()

	dst := cloneToRGBA(src)

	// Resize alpha to match the detected badge dimensions
	alpha := resizeAlpha(srcAlpha, srcW, srcH, aw, ah)

	// Primary strategy: tile-and-reflect texture from directly above the strip.
	// Needs at least a few rows above to sample; otherwise fall back to median.
	if ay >= 6 {
		return textureCopyFill(src, dst, alpha, ax, ay, aw, ah, alphaFloor)
	}

	// Fallback: per-column median from a tall strip above the badge.
	// Used when there isn't enough vertical room to copy a full texture patch.
	sampleHeight := maxInt(60, ah*2)
	sampleY1 := maxInt(0, ay-sampleHeight)

	for col := 0; col < aw; col++ {
		var samplesR, samplesG, samplesB []float64
		px := ax + col
		if px < 0 || px >= imgW {
			continue
		}
		for sy := sampleY1; sy < ay; sy++ {
			off := src.PixOffset(px, sy)
			samplesR = append(samplesR, float64(src.Pix[off+0]))
			samplesG = append(samplesG, float64(src.Pix[off+1]))
			samplesB = append(samplesB, float64(src.Pix[off+2]))
		}
		if len(samplesR) == 0 {
			continue
		}

		bgR := medianFloat(samplesR)
		bgG := medianFloat(samplesG)
		bgB := medianFloat(samplesB)

		// Apply to badge pixels in this column where alpha > alphaFloor
		for row := 0; row < ah; row++ {
			if alpha[row*aw+col] > alphaFloor {
				py := ay + row
				if py < 0 || py >= imgH {
					continue
				}
				off := dst.PixOffset(px, py)
				dst.Pix[off+0] = clampByte(bgR)
				dst.Pix[off+1] = clampByte(bgG)
				dst.Pix[off+2] = clampByte(bgB)
			}
		}
	}

	return dst
}

// textureCopyFill replaces watermark pixels by TILING a small band of texture
// from directly above the strip (with vertical reflection), plus per-channel
// luma correction and top-edge feathering.
//
// Why tile-and-reflect instead of a full-height patch copy: on natural images
// the region directly above the watermark often contains image content that
// transitions right where the watermark begins (e.g. a horizon or gradient).
// Copying ah rows would drag that content into the fill and create a false
// gradient. Using only the last N rows immediately above the strip (the
// "true local background") and reflecting them to fill ah rows gives natural
// texture without dragging distant content along for the ride.
//
// Preconditions:
//   - dst is already a clone of src (this function writes into dst)
//   - alpha has been resized to (aw, ah)
//   - ay >= 1 (there's at least one row above the strip)
func textureCopyFill(src, dst *image.RGBA, alpha []float64, ax, ay, aw, ah int, alphaFloor float64) *image.RGBA {
	b := src.Bounds()
	imgW := b.Dx()
	imgH := b.Dy()

	// Texture band: the last bandHeight rows immediately above the watermark.
	// Small (~ah/3) so we sample only truly-local background, not distant
	// image content that happens to sit farther above.
	bandHeight := maxInt(6, minInt(ah/3, ay))
	bandTop := ay - bandHeight

	// Vertical reflection lookup: fill row -> source y in the band.
	// The band is walked top-down, then bottom-up, then top-down, ... so ah
	// fill rows are covered without a visible periodic seam.
	sourceRow := func(row int) int {
		period := 2 * bandHeight
		p := row % period
		if p < bandHeight {
			return bandTop + p
		}
		return bandTop + (period - 1 - p)
	}

	// Compute per-channel luma offset: match the mean of pixels the tiled
	// texture will contribute to the mean of the band itself. Uniform offset
	// preserves the texture's std (avoiding the "flat color block" look).
	ringHeight := minInt(8, bandHeight)
	var ringR, ringG, ringB, ringN float64
	var patchR, patchG, patchB, patchN float64
	for row := 0; row < ah; row++ {
		for col := 0; col < aw; col++ {
			if alpha[row*aw+col] <= alphaFloor {
				continue
			}
			px := ax + col
			if px < 0 || px >= imgW {
				continue
			}
			sy := sourceRow(row)
			if sy >= 0 && sy < imgH {
				off := src.PixOffset(px, sy)
				patchR += float64(src.Pix[off+0])
				patchG += float64(src.Pix[off+1])
				patchB += float64(src.Pix[off+2])
				patchN++
			}
			for ry := ay - ringHeight; ry < ay; ry++ {
				if ry < 0 || ry >= imgH {
					continue
				}
				off := src.PixOffset(px, ry)
				ringR += float64(src.Pix[off+0])
				ringG += float64(src.Pix[off+1])
				ringB += float64(src.Pix[off+2])
				ringN++
			}
		}
	}
	var dR, dG, dB float64
	if patchN > 0 && ringN > 0 {
		dR = clampFloat(ringR/ringN-patchR/patchN, -25, 25)
		dG = clampFloat(ringG/ringN-patchG/patchN, -25, 25)
		dB = clampFloat(ringB/ringN-patchB/patchN, -25, 25)
	}

	// Copy the patch with per-channel offset. Feather the top rows into the
	// natural image content above so the horizontal seam is invisible.
	featherRows := maxInt(2, minInt(4, ah/8))
	for row := 0; row < ah; row++ {
		py := ay + row
		if py < 0 || py >= imgH {
			continue
		}
		for col := 0; col < aw; col++ {
			if alpha[row*aw+col] <= alphaFloor {
				continue
			}
			px := ax + col
			if px < 0 || px >= imgW {
				continue
			}
			sy := sourceRow(row)
			soff := src.PixOffset(px, sy)
			r := float64(src.Pix[soff+0]) + dR
			g := float64(src.Pix[soff+1]) + dG
			bb := float64(src.Pix[soff+2]) + dB

			// Top-edge feather: blend copied pixel with the pixel just above
			// the strip (natural content) so the horizontal seam vanishes.
			if row < featherRows {
				blend := float64(row+1) / float64(featherRows+1) // 0..1 ramp
				aboveY := ay - featherRows + row
				if aboveY >= 0 && aboveY < ay {
					aoff := src.PixOffset(px, aboveY)
					ar := float64(src.Pix[aoff+0])
					ag := float64(src.Pix[aoff+1])
					ab := float64(src.Pix[aoff+2])
					r = ar*(1-blend) + r*blend
					g = ag*(1-blend) + g*blend
					bb = ab*(1-blend) + bb*blend
				}
			}

			off := dst.PixOffset(px, py)
			dst.Pix[off+0] = clampByte(r)
			dst.Pix[off+1] = clampByte(g)
			dst.Pix[off+2] = clampByte(bb)
		}
	}
	return dst
}

// clampFloat clamps v to [lo, hi].
func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// medianFloat returns the median of a float64 slice.
func medianFloat(s []float64) float64 {
	if len(s) == 0 {
		return 128
	}
	// Simple insertion sort for small slices
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
	return s[len(s)/2]
}
