package watermark

import (
	"image"
	"math"
)

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

		// Phase 1: try multiple alpha gains.
		// Use per-config gains (set by RegisterLearnResult based on watermark type),
		// or fall back to sparkle-safe defaults for manually-built configs.
		gains := cfg.AlphaGains
		if len(gains) == 0 {
			gains = SparkleAlphaGains
		}
		for _, gain := range gains {
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
			// Text watermark residual cleanup: dilate alpha mask + inpaint.
			// Floor 0.01 (not 0.03) ensures anti-aliased glyph edges where
			// the alpha map is faint (0.01–0.03) are still covered — these
			// pixels carry real watermark signal (observed on black/gray
			// seeds where R can be 160+ despite alpha < 0.012) and must
			// not be left as residual bright spots.
			best.dst = inpaintResidual(best.dst, best.alpha, det.x, det.y, dw, dh, 0.01, 7, 3)

			// Full-rectangle fallback: the learned alpha map may not cover
			// the entire watermark footprint — anti-aliased edges where
			// alpha decays to exactly 0 (outside the two-capture method's
			// sensitivity) still carry watermark signal (observed R=156 on
			// black seeds).  Inpaint the entire detected rectangle to
			// guarantee no residual survives inside the watermark bounding
			// box.  This is safe because the rectangle is already known to
			// be the watermark area.
			best.dst = inpaintRect(best.dst, det.x, det.y, dw, dh)
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
