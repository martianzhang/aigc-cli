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
		// Text watermark (Doubao): binary mask extraction + NCC alignment search.
		// This mirrors the reference project's approach:
		// 1. Calculate a generous search box in the bottom-right corner
		// 2. Extract a binary mask of watermark-like pixels (bright, low-sat, tophat)
		// 3. NCC-align the alpha silhouette against the binary mask at multiple scales
		// 4. Use the best match position for removal
		srcW, srcH := alphaData.Width, alphaData.Height
		positions := cfg.PositionResolver(w, h)
		params := paramsForConfig(cfg.Name)

		for _, pos := range positions {
			if pos.W < 16 || pos.H < 10 {
				continue
			}

			// Build a generous search box around the expected position.
			// The actual watermark may be at a different position/scale than
			// the PositionResolver calculates (observed on 2848×1600 images
			// where the watermark scales with height, not width).
			searchPadX := maxInt(60, int(float64(pos.W)*0.5))
			searchPadY := maxInt(30, int(float64(pos.H)*0.5))
			bx := maxInt(0, pos.X-searchPadX)
			by := maxInt(0, pos.Y-searchPadY)
			bw := minInt(w, pos.W+searchPadX*2)
			bh := minInt(h, pos.H+searchPadY*2)
			if bx+bw > w {
				bw = w - bx
			}
			if by+bh > h {
				bh = h - by
			}
			if bw < pos.W+20 || bh < pos.H+10 {
				continue
			}

			// Extract binary mask from the search box
			mask := extractBinaryMask(img, bx, by, bw, bh, params)

			// NCC alignment search: find exact watermark position
			bestX, bestY, bestW, bestH, bestScore := alignByNCC(
				mask, bw, bh, bx, by,
				alphaData.Data, srcW, srcH, pos.W, params,
			)

			if bestScore > 0.05 {
				sz := bestW
				if bestH < sz {
					sz = bestH
				}
				cand := &candidate{
					x:          bestX,
					y:          bestY,
					size:       sz,
					w:          bestW,
					h:          bestH,
					confidence: bestScore,
				}
				seeds = append(seeds, seedScore{bestX, bestY, sz, bestScore, cand})
			} else {
				// Fallback: use PositionResolver position directly (for light/white
				// backgrounds where binary mask extraction can't find the watermark).
				// The TC260 metadata already confirmed this is a Doubao image.
				cand := &candidate{
					x:          pos.X,
					y:          pos.Y,
					size:       minInt(pos.W, pos.H),
					w:          pos.W,
					h:          pos.H,
					confidence: 0.15, // low confidence — fallback, not NCC-verified
				}
				seeds = append(seeds, seedScore{pos.X, pos.Y, minInt(pos.W, pos.H), 0.15, cand})
			}
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
