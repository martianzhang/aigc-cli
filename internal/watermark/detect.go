package watermark

import (
	"image"
	"math"
)

// candidate holds a detection candidate with its scores.
type candidate struct {
	x, y, size int
	w, h       int // actual alpha map size (w=h=size for square alpha maps)
	spatial    float64
	gradient   float64
	variance   float64
	confidence float64
}

// detectWatermark performs catalog-first watermark detection.
// Strategy (matching the reference project):
//
//  1. Resolve seed configs from the Gemini size catalog (exact + projected + fallback)
//  2. Score each seed at its exact bottom-right position
//  3. If the best seed passes threshold + 0.08, return immediately (high confidence)
//  4. Otherwise, do a limited coarse+fine search around the best seed positions
func detectWatermark(img image.Image, cfg Config) *candidate {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 64 || h < 64 {
		return nil
	}

	// Precompute grayscale + gradient once
	gray := toGrayscale(img, w, h)
	grad := sobelMagnitude(gray, w, h)
	alphaData := cfg.AlphaMap
	alphaSize := cfg.DefaultSize

	// 1. Resolve seed positions
	type seedScore struct {
		x, y, size int
		confidence float64
		cand       *candidate
	}
	var seeds []seedScore

	if cfg.PositionResolver != nil {
		// Follow the reference project's _aligned_alpha_map approach:
		//   1. Define a detection box (larger than the alpha map)
		//   2. Extract a binary mask of watermark-like pixels (bright, low-sat)
		//   3. Match the glyph silhouette against this mask using TM_CCOEFF_NORMED
		srcW, srcH := alphaData.Width, alphaData.Height
		positions := cfg.PositionResolver(w, h)

		// Build the glyph silhouette (binary: 1 where alpha > 0.15)
		sil := make([]float64, srcW*srcH)
		for i, v := range alphaData.Data {
			if v > 0.15 {
				sil[i] = 1.0
			}
		}

		for _, pos := range positions {
			if pos.W < 16 || pos.H < 16 {
				continue
			}
			// Compute fixed position score (grayscale NCC, as before)
			rsAlpha := resizeAlpha(alphaData.Data, srcW, srcH, pos.W, pos.H)
			rsGrad := sobelMagnitude(rsAlpha, pos.W, pos.H)
			fixed := scoreCandidateRect(gray, grad, w, h, rsAlpha, rsGrad, pos.X, pos.Y, pos.W, pos.H)

			// Define a detection box around the expected position (wider than alpha map,
			// similar to the reference project's locate() which uses width_frac=0.22 etc.)
			boxPad := int(float64(pos.W) * 0.2) // 20% padding on each side
			bx := maxInt(0, pos.X-boxPad)
			by := maxInt(0, pos.Y-boxPad)
			bw := minInt(w-bx, pos.W+2*boxPad)
			bh := minInt(h-by, pos.H+2*boxPad)
			// Extract a binary mask of watermark-like pixels within the detection box.
			// If box is too small, skip alignment and use fixed position only.
			doAlign := bw >= pos.W+4 && bh >= pos.H+4
			// Criteria (matching the reference project): bright (luma > 150),
			// low-saturation (channel spread < 55), and brighter than local background.
			boxMask := extractTextMask(gray, grad, w, h, bx, by, bw, bh, 150, 55, 12)

			// TM_CCOEFF_NORMED search: match glyph silhouette against box mask
			expectedW := pos.W
			var aligned *candidate
			if doAlign {
				for scale := 0.88; scale <= 1.121; scale += 0.02 {
					sw := int(math.Round(float64(expectedW) * scale))
					sh := int(math.Round(float64(pos.H) * scale))
					if sw < 16 || sh < 16 || sw > bw || sh > bh {
						continue
					}
					// Resize silhouette to this scale
					t := resizeAlpha(sil, srcW, srcH, sw, sh)
					// MatchTemplate equivalent: NCC of silhouette vs mask region
					maxScore, bestOx, bestOy := -1.0, 0, 0
					maxOx := bw - sw
					maxOy := bh - sh
					for oy := 0; oy <= maxOy; oy++ {
						for ox := 0; ox <= maxOx; ox++ {
							// Extract mask region
							maskRegion := make([]float64, sw*sh)
							for row := 0; row < sh; row++ {
								for col := 0; col < sw; col++ {
									maskRegion[row*sw+col] = boxMask[(oy+row)*bw+ox+col]
								}
							}
							score := ncc(maskRegion, t)
							if score > maxScore {
								maxScore = score
								bestOx, bestOy = ox, oy
							}
						}
					}
					if maxScore < 0 {
						continue
					}
					// Compute the absolute position
					absX, absY := bx+bestOx, by+bestOy
					// Score the alpha map at this position using grayscale NCC
					rsA := resizeAlpha(alphaData.Data, srcW, srcH, sw, sh)
					rsG := sobelMagnitude(rsA, sw, sh)
					cand := scoreCandidateRect(gray, grad, w, h, rsA, rsG, absX, absY, sw, sh)
					if cand == nil {
						continue
					}
					cand.x, cand.y = absX, absY
					cand.w, cand.h = sw, sh
					if aligned == nil || cand.confidence > aligned.confidence {
						aligned = cand
					}
				}
			}

			// Pick the better of fixed vs aligned
			best := fixed
			if aligned != nil && (best == nil || aligned.confidence > best.confidence) {
				best = aligned
			}
			if best == nil {
				continue
			}
			best.w, best.h = best.w, best.h
			if best.w <= 0 || best.h <= 0 {
				best.w, best.h = pos.W, pos.H
			}
			sz := best.w
			if best.h < sz {
				sz = best.h
			}
			best.size = sz
			sizeWeight := math.Min(1, math.Cbrt(float64(sz)/float64(srcW)))
			adjusted := best.confidence * sizeWeight
			if adjusted < 0.08 {
				continue
			}
			best.confidence = adjusted
			seeds = append(seeds, seedScore{best.x, best.y, sz, adjusted, best})
		}
	} else {
		// Use Gemini catalog positions
		seedEntries := resolveWatermarkConfigs(w, h)
		for _, entry := range seedEntries {
			sz := entry.logoSize
			cx := w - entry.marginX - sz
			cy := h - entry.marginY - sz
			if cx < 0 || cy < 0 || cx+sz > w || cy+sz > h {
				continue
			}
			if sz < 16 || sz > 192 {
				continue
			}

			rsAlpha := resizeAlpha(alphaData.Data, alphaSize, alphaSize, sz, sz)
			rsGrad := sobelMagnitude(rsAlpha, sz, sz)
			cand := scoreCandidate(gray, grad, w, h, rsAlpha, rsGrad, cx, cy, sz)
			if cand == nil {
				continue
			}
			sizeWeight := math.Min(1, math.Cbrt(float64(sz)/float64(alphaSize)))
			adjusted := cand.confidence * sizeWeight
			if adjusted < 0.08 {
				continue
			}
			cand.confidence = adjusted
			seeds = append(seeds, seedScore{cx, cy, sz, adjusted, cand})
		}
	}

	// Sort seeds by confidence descending
	for i := 0; i < len(seeds); i++ {
		for j := i + 1; j < len(seeds); j++ {
			if seeds[j].confidence > seeds[i].confidence {
				seeds[i], seeds[j] = seeds[j], seeds[i]
			}
		}
	}

	// 3. If best seed passes high threshold, return immediately.
	//    For PositionResolver configs (text watermarks), skip the fine search
	//    entirely — the exact position is known, no need to refine.
	if len(seeds) > 0 && (cfg.PositionResolver != nil || seeds[0].confidence >= cfg.DetectThreshold+0.08) {
		return seeds[0].cand
	}

	// 4. Limited coarse+fine search around the top few seed positions (Gemini only)
	if len(seeds) == 0 || cfg.PositionResolver != nil {
		return nil
	}

	// Determine search bounds from the top 3 seeds (or fewer)
	minX, maxX := w, 0
	minY, maxY := h, 0
	minSize, maxSize := 96, 0
	for i := 0; i < len(seeds) && i < 3; i++ {
		ss := seeds[i]
		if ss.x < minX {
			minX = ss.x
		}
		if ss.x+ss.size > maxX {
			maxX = ss.x + ss.size
		}
		if ss.y < minY {
			minY = ss.y
		}
		if ss.y+ss.size > maxY {
			maxY = ss.y + ss.size
		}
		if ss.size < minSize {
			minSize = ss.size
		}
		if ss.size > maxSize {
			maxSize = ss.size
		}
	}

	// Expand search region by 24px in each direction
	searchLeft := maxInt(0, minX-24)
	searchRight := minInt(w, maxX+24)
	searchTop := maxInt(0, minY-24)
	searchBottom := minInt(h, maxY+24)
	searchMinSize := maxInt(24, minSize-16)
	searchMaxSize := minInt(192, maxSize+16)

	const coarseStride = 8
	var top5 []candidate

	for size := searchMinSize; size <= searchMaxSize && size <= searchRight-searchLeft; size += coarseStride {
		rsAlpha := resizeAlpha(alphaData.Data, alphaSize, alphaSize, size, size)
		rsGrad := sobelMagnitude(rsAlpha, size, size)

		for cx := searchLeft; cx+size <= searchRight; cx += coarseStride {
			for cy := searchTop; cy+size <= searchBottom; cy += coarseStride {
				cand := scoreCandidate(gray, grad, w, h, rsAlpha, rsGrad, cx, cy, size)
				if cand == nil {
					continue
				}
				sizeWeight := math.Min(1, math.Cbrt(float64(size)/float64(alphaSize)))
				adjusted := cand.confidence * sizeWeight
				if adjusted < 0.08 {
					continue
				}
				cand.confidence = adjusted
				insertTop5(&top5, cand, 5)
			}
		}
	}

	if len(top5) == 0 {
		return seeds[0].cand
	}

	// Fine search around top 5
	const fineStride = 2
	const fineRange = 8
	var best *candidate

	for _, coarse := range top5 {
		minFS := maxInt(searchMinSize, coarse.size-fineRange)
		maxFS := minInt(searchMaxSize, coarse.size+fineRange)

		for size := minFS; size <= maxFS; size += fineStride {
			rsAlpha := resizeAlpha(alphaData.Data, alphaSize, alphaSize, size, size)
			rsGrad := sobelMagnitude(rsAlpha, size, size)

			for dx := -fineRange; dx <= fineRange; dx += fineStride {
				cx := coarse.x + dx
				if cx < searchLeft || cx+size > searchRight {
					continue
				}
				for dy := -fineRange; dy <= fineRange; dy += fineStride {
					cy := coarse.y + dy
					if cy < searchTop || cy+size > searchBottom {
						continue
					}
					cand := scoreCandidate(gray, grad, w, h, rsAlpha, rsGrad, cx, cy, size)
					if cand == nil || cand.confidence < cfg.DetectThreshold {
						continue
					}
					if best == nil || cand.confidence > best.confidence {
						best = cand
					}
				}
			}
		}
	}

	if best != nil {
		return best
	}

	// Fall back to best seed
	return seeds[0].cand
}
