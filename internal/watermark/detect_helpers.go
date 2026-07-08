package watermark

import (
	"image"
	"math"
)

// scoreCandidate computes three-stage NCC for a single candidate position.
// size is the square size used for detection (min of w/h for rectangular alpha maps).
func scoreCandidate(gray, grad []float64, imgW, imgH int,
	alpha, alphaGrad []float64, cx, cy, size int) *candidate {

	if cx < 0 || cy < 0 || cx+size > imgW || cy+size > imgH {
		return nil
	}
	if size < 4 {
		return nil
	}

	// Extract region from grayscale and gradient
	// Use the square size for detection (alpha and alphaGrad are also size×size)
	grayRegion := make([]float64, size*size)
	gradRegion := make([]float64, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			gi := (cy+y)*imgW + cx + x
			li := y*size + x
			grayRegion[li] = gray[gi]
			gradRegion[li] = grad[gi]
		}
	}

	// Spatial NCC: grayscale vs alpha map
	spatial := ncc(grayRegion, alpha)

	// Gradient NCC: edges vs alpha map edges
	gradientScore := ncc(gradRegion, alphaGrad)

	// Variance analysis: watermark region should have lower variance
	varianceScore := 0.5 // neutral default
	refY := cy - size
	if refY >= 0 {
		refH := minInt(size, cy-refY)
		if refH > 8 {
			wmVar := regionVariance(gray, imgW, cx, cy, size, size)
			refVar := regionVariance(gray, imgW, cx, refY, size, refH)
			if refVar > 1e-10 {
				v := 1 - wmVar/refVar
				if v > 0 {
					varianceScore = v
				}
				if varianceScore > 1 {
					varianceScore = 1
				}
			}
		}
	}

	// Three-stage weighted fusion
	if spatial < 0 {
		spatial = 0
	}
	if gradientScore < 0 {
		gradientScore = 0
	}
	confidence := spatial*0.5 + gradientScore*0.3 + varianceScore*0.2

	return &candidate{
		x: cx, y: cy, size: size,
		spatial:    spatial,
		gradient:   gradientScore,
		variance:   varianceScore,
		confidence: confidence,
	}
}

// scoreCandidateRect scores a rectangular region with a rectangular alpha map.
// Used for text watermarks (Doubao, Jimeng) where the alpha map is wider than tall.
// The alpha and alphaGrad should already be resized to aw×ah.
func scoreCandidateRect(gray, grad []float64, imgW, imgH int,
	alpha, alphaGrad []float64, cx, cy, aw, ah int) *candidate {

	if cx < 0 || cy < 0 || cx+aw > imgW || cy+ah > imgH {
		return nil
	}
	if aw < 4 || ah < 4 {
		return nil
	}

	// Extract rectangular region from grayscale and gradient
	grayRegion := make([]float64, aw*ah)
	gradRegion := make([]float64, aw*ah)
	for y := 0; y < ah; y++ {
		for x := 0; x < aw; x++ {
			gi := (cy+y)*imgW + cx + x
			li := y*aw + x
			grayRegion[li] = gray[gi]
			gradRegion[li] = grad[gi]
		}
	}

	// Spatial NCC: grayscale vs alpha map (both are aw×ah)
	spatial := ncc(grayRegion, alpha)

	// Gradient NCC: edges vs alpha map edges
	gradientScore := ncc(gradRegion, alphaGrad)

	// Variance analysis
	varianceScore := 0.5
	refY := cy - ah
	if refY >= 0 {
		refH := minInt(ah, cy-refY)
		if refH > 8 {
			wmVar := regionVariance(gray, imgW, cx, cy, aw, ah)
			refVar := regionVariance(gray, imgW, cx, refY, aw, refH)
			if refVar > 1e-10 {
				v := 1 - wmVar/refVar
				if v > 0 {
					varianceScore = v
				}
				if varianceScore > 1 {
					varianceScore = 1
				}
			}
		}
	}

	if spatial < 0 {
		spatial = 0
	}
	if gradientScore < 0 {
		gradientScore = 0
	}
	confidence := spatial*0.5 + gradientScore*0.3 + varianceScore*0.2

	// Use min dimension as size for compatibility
	sz := aw
	if ah < sz {
		sz = ah
	}
	return &candidate{
		x: cx, y: cy, size: sz, w: aw, h: ah,
		spatial:    spatial,
		gradient:   gradientScore,
		variance:   varianceScore,
		confidence: confidence,
	}
}

// toGrayscale converts an image to normalized [0,1] float64 grayscale.
func toGrayscale(img image.Image, w, h int) []float64 {
	gray := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			lum := 0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(b>>8)
			gray[y*w+x] = lum / 255.0
		}
	}
	return gray
}

// sobelMagnitude computes 3×3 Sobel gradient magnitude.
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

// ncc computes normalized cross-correlation between two float64 slices.
func ncc(a, b []float64) float64 {
	n := len(a)
	if n == 0 || n != len(b) {
		return 0
	}
	var sumA, sumB float64
	for i := 0; i < n; i++ {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)
	var num, denA, denB float64
	for i := 0; i < n; i++ {
		da := a[i] - meanA
		db := b[i] - meanB
		num += da * db
		denA += da * da
		denB += db * db
	}
	den := math.Sqrt(denA * denB)
	if den < 1e-10 {
		return 0
	}
	return num / den
}

// regionVariance computes the variance of a rectangular region in grayscale.
func regionVariance(gray []float64, stride int, x, y, w, h int) float64 {
	var sum, sumSq, n float64
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			v := gray[(y+dy)*stride+x+dx]
			sum += v
			sumSq += v * v
			n++
		}
	}
	if n == 0 {
		return 0
	}
	mean := sum / n
	return sumSq/n - mean*mean
}

// resizeAlpha bilinearly resizes an alpha map from (sw, sh) to (dw, dh).
func resizeAlpha(src []float64, sw, sh, dw, dh int) []float64 {
	if dw == sw && dh == sh {
		dst := make([]float64, dw*dh)
		copy(dst, src)
		return dst
	}
	dst := make([]float64, dw*dh)
	for dy := 0; dy < dh; dy++ {
		sy := float64(dy) * float64(sh-1) / float64(maxInt(dh-1, 1))
		iy0 := int(sy)
		iy1 := minInt(iy0+1, sh-1)
		fy := sy - float64(iy0)
		for dx := 0; dx < dw; dx++ {
			sx := float64(dx) * float64(sw-1) / float64(maxInt(dw-1, 1))
			ix0 := int(sx)
			ix1 := minInt(ix0+1, sw-1)
			fx := sx - float64(ix0)

			v00 := src[iy0*sw+ix0]
			v10 := src[iy0*sw+ix1]
			v01 := src[iy1*sw+ix0]
			v11 := src[iy1*sw+ix1]

			top := v00*(1-fx) + v10*fx
			bot := v01*(1-fx) + v11*fx
			dst[dy*dw+dx] = top*(1-fy) + bot*fy
		}
	}
	return dst
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
