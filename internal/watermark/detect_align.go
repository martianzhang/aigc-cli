package watermark

import (
	"math"
)


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
