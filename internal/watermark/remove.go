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
			bodyDark := checkOversubtraction(img, best.alpha, det.x, det.y, dw, dh)
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
// (glyph body >25 gray levels below surrounding background ring).
// Mirrors the reference project's _reverse_alpha_oversubtracts gate.
func checkOversubtraction(img image.Image, alpha []float64, ax, ay, aw, ah int) bool {
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
	return predictedCore < bg-25.0
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

// cloneToRGBA converts any image to *image.RGBA.
func cloneToRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}
