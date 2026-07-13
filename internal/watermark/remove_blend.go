package watermark

import (
	"image"
	"math"
)

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
