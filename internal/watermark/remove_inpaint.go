package watermark

import (
	"image"
	"image/draw"
)
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

// inpaintRect replaces all pixels in the rectangular region (ax,ay,aw,ah)
// with progressive boundary-growing inpaint from the surrounding area.
// Unlike inpaintResidual which uses an alpha-gated mask, this covers the
// ENTIRE rectangle — used as a fallback for text watermarks where the
// learned alpha map has zero-valued edges that still carry watermark signal.
func inpaintRect(src *image.RGBA, ax, ay, aw, ah int) *image.RGBA {
	b := src.Bounds()
	imgW, imgH := b.Dx(), b.Dy()

	// Build mask covering the full rectangle
	mask := make([]bool, imgW*imgH)
	for y := ay; y < ay+ah && y < imgH; y++ {
		if y < 0 {
			continue
		}
		for x := ax; x < ax+aw && x < imgW; x++ {
			if x < 0 {
				continue
			}
			mask[y*imgW+x] = true
		}
	}

	dst := cloneToRGBA(src)

	// Progressive boundary-growing inpaint (same algorithm as inpaintResidual)
	for {
		type bp struct{ x, y int }
		var boundary []bp
		for y := 0; y < imgH; y++ {
			for x := 0; x < imgW; x++ {
				if !mask[y*imgW+x] {
					continue
				}
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
			break
		}

		for _, p := range boundary {
			x, y := p.x, p.y
			var sumR, sumG, sumB, sumW float64
			for dy := -3; dy <= 3; dy++ {
				for dx := -3; dx <= 3; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if nx < 0 || nx >= imgW || ny < 0 || ny >= imgH {
						continue
					}
					if mask[ny*imgW+nx] {
						continue
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
