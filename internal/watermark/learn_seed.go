package watermark

import (
	"image"
	"math"
)
type bgModel struct {
	baseR, baseG, baseB float64
	gxR, gxG, gxB       float64 // horizontal gradient (per column)
	gyR, gyG, gyB       float64 // vertical gradient (per row)
	lumStd              float64 // noise level for data-driven floor
}

// bgAt returns the interpolated background RGB at pixel (x, y).
// Values are clamped to [0, 255] to prevent gradient over-extrapolation
// from producing negative backgrounds (which would amplify watermark signal).
func (m *bgModel) bgAt(x, y int) (r, g, b float64) {
	fx := float64(x)
	fy := float64(y)
	r = clampBG(m.baseR + m.gxR*fx + m.gyR*fy)
	g = clampBG(m.baseG + m.gxG*fx + m.gyG*fy)
	b = clampBG(m.baseB + m.gxB*fx + m.gyB*fy)
	return
}

// clampBG clamps a background value to [0, 255].
func clampBG(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// calibrateBackground builds per-pixel gradient models for both seed images
// by sampling the top edge (first 6 rows) and left edge (first 6 columns).
// This removes brightness gradients that some platforms introduce.
func calibrateBackground(black, gray image.Image, w, h int) (blackModel, grayModel bgModel) {
	b := meanEdges(black, w, h)
	g := meanEdges(gray, w, h)
	blackModel = fitLinear(b.top, b.left, w, h)
	grayModel = fitLinear(g.top, g.left, w, h)
	// Noise floor: use the LARGER of corner-only noise and edge-residual
	// noise.  Corner-only (std in 40×40 top-left) captures JPEG noise.
	// Edge-residual (std after subtracting gradient model from edge strips)
	// captures gradient-model imperfections.  Using max() means clean
	// uniform seeds use cornerStd, while gradient seeds use edgeNoise.
	cStdB := cornerStd(black, w, h)
	cStdG := cornerStd(gray, w, h)
	eNoiseB := edgeNoise(black, w, h, blackModel)
	eNoiseG := edgeNoise(gray, w, h, grayModel)
	blackModel.lumStd = maxFloat(cStdB, eNoiseB)
	grayModel.lumStd = maxFloat(cStdG, eNoiseG)
	return
}

type edgeMeans struct {
	top  [][3]float64 // per-column RGB means from top edge (rows 0-5)
	left [][3]float64 // per-row RGB means from left edge (cols 0-5)
}

func meanEdges(img image.Image, w, h int) edgeMeans {
	const stripH = 6
	const stripW = 6

	top := make([][3]float64, w)
	for x := 0; x < w; x++ {
		var sr, sg, sb float64
		for y := 0; y < stripH && y < h; y++ {
			r, g, b, _ := img.At(x, y).RGBA()
			sr += float64(r >> 8)
			sg += float64(g >> 8)
			sb += float64(b >> 8)
		}
		n := float64(minInt(stripH, h))
		top[x] = [3]float64{sr / n, sg / n, sb / n}
	}

	left := make([][3]float64, h)
	for y := 0; y < h; y++ {
		var sr, sg, sb float64
		for x := 0; x < stripW && x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			sr += float64(r >> 8)
			sg += float64(g >> 8)
			sb += float64(b >> 8)
		}
		n := float64(minInt(stripW, w))
		left[y] = [3]float64{sr / n, sg / n, sb / n}
	}

	return edgeMeans{top: top, left: left}
}

// fitLinear fits bg(x,y) = base + gx*col + gy*row using the top edge
// (y≈0, shows horizontal gradient) and left edge (x≈0, shows vertical gradient).
// Uses a robust mean-of-differences estimator: compares the average of the
// first and last N pixels to compute the gradient, which is far less sensitive
// to noise than per-pixel division.
func fitLinear(top, left [][3]float64, w, h int) bgModel {
	var m bgModel
	if len(top) == 0 || len(left) == 0 {
		return m
	}
	const nAvg = 30 // number of pixels averaged at each end

	// base from top-left corner
	m.baseR = (top[0][0] + left[0][0]) / 2
	m.baseG = (top[0][1] + left[0][1]) / 2
	m.baseB = (top[0][2] + left[0][2]) / 2

	// Horizontal gradient from top edge: (end_avg - start_avg) / span
	nx := minInt(nAvg, len(top)/2)
	if nx > 1 {
		var sR, sG, sB, eR, eG, eB float64
		for i := 0; i < nx; i++ {
			sR += top[i][0]
			sG += top[i][1]
			sB += top[i][2]
			eR += top[len(top)-1-i][0]
			eG += top[len(top)-1-i][1]
			eB += top[len(top)-1-i][2]
		}
		fn := float64(nx)
		span := float64(len(top) - nx)
		m.gxR = (eR/fn - sR/fn) / span
		m.gxG = (eG/fn - sG/fn) / span
		m.gxB = (eB/fn - sB/fn) / span
	}

	// Vertical gradient from left edge
	ny := minInt(nAvg, len(left)/2)
	if ny > 1 {
		var sR, sG, sB, eR, eG, eB float64
		for i := 0; i < ny; i++ {
			sR += left[i][0]
			sG += left[i][1]
			sB += left[i][2]
			eR += left[len(left)-1-i][0]
			eG += left[len(left)-1-i][1]
			eB += left[len(left)-1-i][2]
		}
		fn := float64(ny)
		span := float64(len(left) - ny)
		m.gyR = (eR/fn - sR/fn) / span
		m.gyG = (eG/fn - sG/fn) / span
		m.gyB = (eB/fn - sB/fn) / span
	}

	return m
}

// edgeNoise computes the residual noise level from the top and left edge strips
// after subtracting the linear gradient model.  This gives a more accurate noise
// estimate than the top-left corner alone, because it captures gradient-model
// imperfections that some platforms' seeds exhibit.
func edgeNoise(img image.Image, w, h int, m bgModel) float64 {
	const strip = 6
	var sumSq float64
	var n float64
	for y := 0; y < strip && y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			lum := (float64(r>>8) + float64(g>>8) + float64(b>>8)) / 3.0
			mr, mg, mb := m.bgAt(x, y)
			bgLum := (mr + mg + mb) / 3.0
			residual := lum - bgLum
			sumSq += residual * residual
			n++
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < strip && x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			lum := (float64(r>>8) + float64(g>>8) + float64(b>>8)) / 3.0
			mr, mg, mb := m.bgAt(x, y)
			bgLum := (mr + mg + mb) / 3.0
			residual := lum - bgLum
			sumSq += residual * residual
			n++
		}
	}
	if n < 2 {
		return 0
	}
	return math.Sqrt(sumSq / n)
}

// cornerStd computes the luminance standard deviation in the top-left 40×40 corner.
func cornerStd(img image.Image, w, h int) float64 {
	sampleW := minInt(40, w)
	sampleH := minInt(40, h)
	var sum, sumSq float64
	var n float64
	for y := 0; y < sampleH; y++ {
		for x := 0; x < sampleW; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			lum := (float64(r>>8) + float64(g>>8) + float64(b>>8)) / 3.0
			sum += lum
			sumSq += lum * lum
			n++
		}
	}
	if n < 2 {
		return 0
	}
	mean := sum / n
	return math.Sqrt((sumSq - n*mean*mean) / (n - 1))
}

// SeedQualityLevel describes the quality of a seed image metric.
type SeedQualityLevel int

const (
	SeedGood SeedQualityLevel = iota
	SeedWarn
	SeedFail
)

func (l SeedQualityLevel) String() string {
	switch l {
	case SeedGood:
		return "GOOD"
	case SeedWarn:
		return "WARN"
	case SeedFail:
		return "FAIL"
	default:
		return "?"
	}
}

// SeedQuality holds quality assessment results for a pair of seed images.
type SeedQuality struct {
	BlackBG       float64          // mean luminance of black seed top-left
	GrayBG        float64          // mean luminance of gray seed top-left
	BlackStd      float64          // std of black seed top-left
	GrayStd       float64          // std of gray seed top-left
	BlackNoise    float64          // edge noise after gradient removal (black)
	GrayNoise     float64          // edge noise after gradient removal (gray)
	Gx, Gy        float64          // gradient magnitude
	SignalMax     float64          // max corrected signal in bottom-right 200x200
	BGScore       SeedQualityLevel // black~0?, gray~128?
	GradientScore SeedQualityLevel // gradient small?
	NoiseScore    SeedQualityLevel // noise level acceptable?
	SignalScore   SeedQualityLevel // watermark signal present?
}

// AssessSeedQuality evaluates seed image quality for the two-capture method.
func AssessSeedQuality(black, gray image.Image) SeedQuality {
	b := black.Bounds()
	w, h := b.Dx(), b.Dy()

	blackModel, grayModel := calibrateBackground(black, gray, w, h)

	// Background luminance
	bLum := cornerLum(black, w, h)
	gLum := cornerLum(gray, w, h)

	// Noise after gradient correction
	bNoise := edgeNoise(black, w, h, blackModel)
	gNoise := edgeNoise(gray, w, h, grayModel)

	// Gradient magnitude
	gx := (blackModel.gxR + blackModel.gxG + blackModel.gxB + grayModel.gxR + grayModel.gxG + grayModel.gxB) / 6.0
	gy := (blackModel.gyR + blackModel.gyG + blackModel.gyB + grayModel.gyR + grayModel.gyG + grayModel.gyB) / 6.0

	// Watermark signal: max corrected luminance in bottom-right 200x200
	var signalMax float64
	const signalRegion = 200
	by := maxInt(0, h-signalRegion)
	bx := maxInt(0, w-signalRegion)
	for y := by; y < h; y++ {
		for x := bx; x < w; x++ {
			br, bg, bb, _ := black.At(x, y).RGBA()
			gr, gg, gb, _ := gray.At(x, y).RGBA()
			bL := (float64(br>>8) + float64(bg>>8) + float64(bb>>8)) / 3.0
			gL := (float64(gr>>8) + float64(gg>>8) + float64(gb>>8)) / 3.0
			bBR, bBG, bBB := blackModel.bgAt(x, y)
			gGR, gGG, gGB := grayModel.bgAt(x, y)
			bCorr := bL - (bBR+bBG+bBB)/3.0
			gCorr := gL - (gGR+gGG+gGB)/3.0
			signal := (bCorr + gCorr) / 2.0
			if signal > signalMax {
				signalMax = signal
			}
		}
	}

	q := SeedQuality{
		BlackBG:    bLum,
		GrayBG:     gLum,
		BlackStd:   cornerStd(black, w, h),
		GrayStd:    cornerStd(gray, w, h),
		BlackNoise: bNoise,
		GrayNoise:  gNoise,
		Gx:         gx,
		Gy:         gy,
		SignalMax:  signalMax,
	}

	// Background score
	if bLum < 5 && gLum > 115 && gLum < 140 {
		q.BGScore = SeedGood
	} else if bLum < 15 || (gLum > 100 && gLum < 160) {
		q.BGScore = SeedWarn
	} else {
		q.BGScore = SeedFail
	}

	// Gradient score
	gradMag := math.Sqrt(gx*gx + gy*gy)
	if gradMag < 0.01 {
		q.GradientScore = SeedGood
	} else if gradMag < 0.05 {
		q.GradientScore = SeedWarn
	} else {
		q.GradientScore = SeedFail
	}

	// Noise score (use larger of the two)
	noise := maxFloat(bNoise, gNoise)
	if noise < 5 {
		q.NoiseScore = SeedGood
	} else if noise < 15 {
		q.NoiseScore = SeedWarn
	} else {
		q.NoiseScore = SeedFail
	}

	// Signal score
	if signalMax > 50 {
		q.SignalScore = SeedGood
	} else if signalMax > 20 {
		q.SignalScore = SeedWarn
	} else {
		q.SignalScore = SeedFail
	}

	return q
}

// cornerLum returns the mean luminance in the top-left 40x40 area.
func cornerLum(img image.Image, w, h int) float64 {
	var sum, n float64
	for y := 0; y < minInt(40, h); y++ {
		for x := 0; x < minInt(40, w); x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			sum += (float64(r>>8) + float64(g>>8) + float64(b>>8)) / 3.0
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

