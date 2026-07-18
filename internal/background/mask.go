// Package background 的 alpha 遮罩生成与后处理。
//
// buildAlphaMask / buildAlphaMaskMulti：核心遮罩生成函数。
// 对每个像素计算与背景色的 ΔE，用三段式映射：
//
//	ΔE ≤ tolerance        → alpha = 0   (透明，确定背景)
//	ΔE ≥ tolerance × 1.5  → alpha = 255 (不透明，确定前景)
//	中间值                 → 线性过渡，用于边缘羽化
//
// 后处理管线（postProcessAlpha）：
//  1. close (形态学闭合)：先膨胀再腐蚀，填掉主体内部的"小洞"
//  2. smooth (3×3 均值滤波)：去除遮罩噪点
//  3. feather (box blur)：边缘扩散，消除锯齿
//  4. erode (腐蚀)：从边缘向内收缩，去除背景光晕
//
// edgeRefineAlpha：用 Sobel 梯度检测图片强边缘，对过渡区像素
// 做梯度感知的二值化——强边缘处 alpha 推向 0/255，弱边缘保持平滑。
package background

import "math"

// clampByte clamps a float64 to [0, 255] uint8.
func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// buildAlphaMask generates an alpha channel based on ΔE distance from the background color.
// Pixels within tolerance → transparent; above tolerance * fgThreshold → opaque;
// in between → linear transition for feathering.
//
// Returns the alpha channel as a flat []uint8 (0=transparent, 255=opaque).
func buildAlphaMask(rgba []uint8, w, h int, bgL, bga, bgb, tolerance, fgThreshold float64) []uint8 {
	alpha := make([]uint8, w*h)

	if tolerance <= 0 {
		tolerance = 10
	}
	if fgThreshold <= 1 {
		fgThreshold = 1.5
	}

	fgBoundary := tolerance * fgThreshold
	range_ := fgBoundary - tolerance
	if range_ <= 0 {
		range_ = 1
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bb, _ := getPixelRGBA(rgba, w, h, x, y)
			pL, pa, pb := rgbToLab(r, g, bb)
			de := deltaE(pL, pa, pb, bgL, bga, bgb)

			var al float64
			if de <= tolerance {
				al = 0
			} else if de >= fgBoundary {
				al = 255
			} else {
				// Linear transition
				t := (de - tolerance) / range_
				al = t * 255
			}

			alpha[y*w+x] = clampByte(al)
		}
	}

	return alpha
}

// buildAlphaMaskMulti generates alpha using min ΔE across multiple background colors.
// Handles gradients and multi-colored backgrounds better than single-color mode.
func buildAlphaMaskMulti(rgba []uint8, w, h int, bgLabs [][3]float64, tolerance, fgThreshold float64) []uint8 {
	alpha := make([]uint8, w*h)

	if tolerance <= 0 {
		tolerance = 10
	}
	if fgThreshold <= 1 {
		fgThreshold = 1.5
	}

	fgBoundary := tolerance * fgThreshold
	range_ := fgBoundary - tolerance
	if range_ <= 0 {
		range_ = 1
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bb, _ := getPixelRGBA(rgba, w, h, x, y)
			pL, pa, pb := rgbToLab(r, g, bb)

			// Min ΔE to any background color
			minDE := math.MaxFloat64
			for _, bg := range bgLabs {
				de := deltaE(pL, pa, pb, bg[0], bg[1], bg[2])
				if de < minDE {
					minDE = de
				}
			}

			var al float64
			if minDE <= tolerance {
				al = 0
			} else if minDE >= fgBoundary {
				al = 255
			} else {
				t := (minDE - tolerance) / range_
				al = t * 255
			}

			alpha[y*w+x] = clampByte(al)
		}
	}

	return alpha
}

// edgeRefineAlpha uses Sobel gradient of the input image to refine the alpha mask.
// Where the image has strong edges, alpha is pushed towards 0 or 255 (sharp).
// Where the image has weak/no edges, alpha is kept smooth (original feathering).
func edgeRefineAlpha(alpha []uint8, grayPixels []float64, w, h int) []uint8 {
	// Compute Sobel gradient magnitude
	grad := sobelMagnitude(grayPixels, w, h)

	// Normalize gradient to [0, 1]
	maxG := 0.0
	for _, g := range grad {
		if g > maxG {
			maxG = g
		}
	}
	if maxG > 0 {
		for i := range grad {
			grad[i] /= maxG
		}
	}

	out := make([]uint8, w*h)
	copy(out, alpha)

	for i := 0; i < w*h; i++ {
		a := alpha[i]

		// Skip fully transparent or fully opaque pixels
		if a <= 8 || a >= 248 {
			continue
		}

		edge := grad[i]
		if edge < 0.05 {
			continue // weak edge, keep feathered
		}

		// Strong edge: blend between original alpha and nearest extreme
		// Sharpness scales with edge strength
		target := uint8(0)
		if a > 128 {
			target = 255
		}
		blend := edge * 0.8 // max 80% towards extreme
		out[i] = uint8(float64(a)*(1-blend) + float64(target)*blend)
	}

	return out
}

// sobelMagnitude computes 3x3 Sobel gradient magnitude on a grayscale float64 buffer.
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

// featherAlpha applies a simple box blur to the alpha mask for edge softening.
// radius=0 means no feathering.
func featherAlpha(alpha []uint8, w, h, radius int) []uint8 {
	if radius <= 0 {
		return alpha
	}

	// Use a simple separable box blur (fast).
	// First pass: horizontal
	tmp := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var sum float64
			var count int
			x0 := x - radius
			if x0 < 0 {
				x0 = 0
			}
			x1 := x + radius
			if x1 >= w {
				x1 = w - 1
			}
			for cx := x0; cx <= x1; cx++ {
				sum += float64(alpha[y*w+cx])
				count++
			}
			tmp[y*w+x] = sum / float64(count)
		}
	}

	// Second pass: vertical
	out := make([]uint8, w*h)
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			var sum float64
			var count int
			y0 := y - radius
			if y0 < 0 {
				y0 = 0
			}
			y1 := y + radius
			if y1 >= h {
				y1 = h - 1
			}
			for cy := y0; cy <= y1; cy++ {
				sum += tmp[cy*w+x]
				count++
			}
			out[y*w+x] = clampByte(math.Round(sum / float64(count)))
		}
	}

	return out
}

// erodeAlpha applies morphological erosion to the alpha mask.
// Each pixel becomes the minimum value in its radius neighborhood.
// This shrinks the foreground region, removing background fringe.
func erodeAlpha(alpha []uint8, w, h, radius int) []uint8 {
	if radius <= 0 {
		return alpha
	}

	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			minVal := uint8(255)
			y0 := y - radius
			if y0 < 0 {
				y0 = 0
			}
			y1 := y + radius
			if y1 >= h {
				y1 = h - 1
			}
			x0 := x - radius
			if x0 < 0 {
				x0 = 0
			}
			x1 := x + radius
			if x1 >= w {
				x1 = w - 1
			}
			for cy := y0; cy <= y1; cy++ {
				for cx := x0; cx <= x1; cx++ {
					if alpha[cy*w+cx] < minVal {
						minVal = alpha[cy*w+cx]
					}
				}
			}
			out[y*w+x] = minVal
		}
	}
	return out
}

// smoothAlpha applies a lightweight smoothing pass (3×3 mean filter).
func smoothAlpha(alpha []uint8, w, h, passes int) []uint8 {
	if passes <= 0 {
		return alpha
	}

	current := alpha
	for p := 0; p < passes; p++ {
		next := make([]uint8, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var sum float64
				var count int
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						cx, cy := x+dx, y+dy
						if cx >= 0 && cx < w && cy >= 0 && cy < h {
							sum += float64(current[cy*w+cx])
							count++
						}
					}
				}
				next[y*w+x] = clampByte(math.Round(sum / float64(count)))
			}
		}
		current = next
	}
	return current
}

// dilateAlpha applies morphological dilation (max filter).
func dilateAlpha(alpha []uint8, w, h, radius int) []uint8 {
	if radius <= 0 {
		return alpha
	}
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			maxVal := uint8(0)
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					cx, cy := x+dx, y+dy
					if cx >= 0 && cx < w && cy >= 0 && cy < h {
						if alpha[cy*w+cx] > maxVal {
							maxVal = alpha[cy*w+cx]
						}
					}
				}
			}
			out[y*w+x] = maxVal
		}
	}
	return out
}

// closeAlpha applies morphological closing (dilate then erode).
// Fills small holes in the foreground while preserving overall shape.
func closeAlpha(alpha []uint8, w, h, radius int) []uint8 {
	if radius <= 0 {
		return alpha
	}
	dilated := dilateAlpha(alpha, w, h, radius)
	return erodeAlpha(dilated, w, h, radius)
}

// applyAlpha creates an RGBA image from raw pixel data and an alpha channel.
func applyAlpha(rgba []uint8, w, h int, alpha []uint8) []uint8 {
	out := make([]uint8, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w*4 + x*4
			r := rgba[idx]
			g := rgba[idx+1]
			b := rgba[idx+2]
			a := alpha[y*w+x]
			out[idx] = r
			out[idx+1] = g
			out[idx+2] = b
			out[idx+3] = a
		}
	}
	return out
}

// autoFeatherRadius computes an appropriate feather radius based on image size.
func autoFeatherRadius(w, h int) int {
	diag := math.Sqrt(float64(w*w + h*h))
	r := int(math.Round(diag * 0.003))
	if r < 1 {
		return 1
	}
	if r > 5 {
		return 5
	}
	return r
}
