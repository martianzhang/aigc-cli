package watermark

import (
	"image"
	"math"
)

type TextMarkParams struct {
	MaxSaturation  float64 // max RGB channel spread to count as "grayish"
	LogoMinLuma    float64 // minimum absolute brightness for watermark pixels
	TophatDelta    float64 // minimum brightness above local background
	MorphOpenSize  int     // morphological open kernel side
	AlignSearchMin float64 // minimum scale for NCC alignment search
	AlignSearchMax float64 // maximum scale for NCC alignment search
}

// DefaultDoubaoParams returns the reference project's Doubao tuning.
// Text watermarks (doubao/jimeng) scale with image dimensions, but the
// exact scaling behavior varies across model versions and image aspect
// ratios.  A wide alignment search (0.50–2.00) ensures the NCC can find
// the watermark even when PositionResolver's estimate is significantly off.
func DefaultDoubaoParams() TextMarkParams {
	return TextMarkParams{
		MaxSaturation:  55,
		LogoMinLuma:    150,
		TophatDelta:    12,
		MorphOpenSize:  5,
		AlignSearchMin: 0.50,
		AlignSearchMax: 2.00,
	}
}

// DefaultJimengParams returns the reference project's Jimeng tuning.
// Same mask extraction params as Doubao (both ByteDance text marks).
// The wide alignment search (0.50–2.00) handles unknown scaling behavior:
// Jimeng watermarks have been observed to scale with image height rather
// than width on some outputs, causing PositionResolver's width-based
// estimate to be off by up to 2×.
func DefaultJimengParams() TextMarkParams {
	return TextMarkParams{
		MaxSaturation:  55,
		LogoMinLuma:    150,
		TophatDelta:    12,
		MorphOpenSize:  5,
		AlignSearchMin: 0.50,
		AlignSearchMax: 2.00,
	}
}

// paramsForConfig returns the TextMarkParams for a given config name.
func paramsForConfig(name string) TextMarkParams {
	switch name {
	case "jimeng":
		return DefaultJimengParams()
	case "doubao-snap", "baidu":
		return DefaultBadgeParams()
	default:
		return DefaultDoubaoParams()
	}
}

// DefaultBadgeParams returns params for UI badge watermarks (screenshots).
// The badge text is white on dark background, so we use a wider search range
// for DPI variations and a lower tophat threshold since text is high-contrast.
func DefaultBadgeParams() TextMarkParams {
	return TextMarkParams{
		MaxSaturation:  80,   // wider saturation tolerance for badge text
		LogoMinLuma:    60,   // lower brightness threshold (badge text may be dimmer)
		TophatDelta:    8,    // lower tophat threshold (high contrast on dark bg)
		MorphOpenSize:  3,    // smaller kernel for badge text
		AlignSearchMin: 0.50, // wider range for DPI variations
		AlignSearchMax: 2.00,
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