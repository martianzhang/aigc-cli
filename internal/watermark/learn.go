package watermark

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// PNG tEXt metadata keys
const (
	txtName               = "name"
	txtNativeWidth        = "native_width"
	txtMarginXFrac        = "margin_x_frac"
	txtMarginYFrac        = "margin_y_frac"
	txtDetectThreshold    = "detect_threshold"
	txtRemoveStrategy     = "remove_strategy"
	txtOversubtractMargin = "oversubtract_margin"
)

// bgModel describes a per-pixel linear background model:
// bg(channel, x, y) = base + gx*col + gy*row.
// This handles the brightness gradients present in some platforms' seed images.
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

// computeAlphaWith solves the alpha map using a specific background model.
// Shared core used by both the constant-model and gradient-model paths.
func computeAlphaWith(black, gray image.Image, w, h int,
	bgB, bgG *bgModel,
	noiseFloorB, noiseFloorG, noiseGate float64,
	gradient bool) *AlphaMap {

	data := make([]float64, w*h)
	for y := 0; y < h; y++ {
		var bgBR, bgBG, bgBB, bgGR, bgGG, bgGB float64
		if gradient {
			bgBR, bgBG, bgBB = bgB.bgAt(0, y)
			bgGR, bgGG, bgGB = bgG.bgAt(0, y)
		} else {
			bgBR, bgBG, bgBB = bgB.baseR, bgB.baseG, bgB.baseB
			bgGR, bgGG, bgGB = bgG.baseR, bgG.baseG, bgG.baseB
		}

		for x := 0; x < w; x++ {
			if gradient && x > 0 {
				bgBR += bgB.gxR
				bgBG += bgB.gxG
				bgBB += bgB.gxB
				bgGR += bgG.gxR
				bgGG += bgG.gxG
				bgGB += bgG.gxB
			}

			br, bgg, bb, _ := black.At(x, y).RGBA()
			gr, gg, gb, _ := gray.At(x, y).RGBA()

			bR := float64(br >> 8)
			bG := float64(bgg >> 8)
			bB := float64(bb >> 8)
			gR := float64(gr >> 8)
			gG := float64(gg >> 8)
			gB := float64(gb >> 8)

			denBX := 255.0 - bgBR
			denGX := 255.0 - bgGR
			if denBX < 1 {
				denBX = 1
			}
			if denGX < 1 {
				denGX = 1
			}

			bDiffR := maxFloat(0, bR-bgBR-noiseFloorB)
			gDiffR := maxFloat(0, gR-bgGR-noiseFloorG)
			bDiffG := maxFloat(0, bG-bgBG-noiseFloorB)
			gDiffG := maxFloat(0, gG-bgGG-noiseFloorG)
			bDiffB := maxFloat(0, bB-bgBB-noiseFloorB)
			gDiffB := maxFloat(0, gB-bgGB-noiseFloorG)

			alpha := (bDiffR/denBX + gDiffR/denGX +
				bDiffG/denBX + gDiffG/denGX +
				bDiffB/denBX + gDiffB/denGX) / 6.0
			if alpha > 1 {
				alpha = 1
			}
			data[y*w+x] = alpha
		}
	}

	// Spatial smoothing
	smoothed := gaussianBlur(data, w, h, 1.0)

	// Noise gate
	for i, v := range smoothed {
		if v < noiseGate {
			smoothed[i] = 0
		}
	}

	return &AlphaMap{Width: w, Height: h, Data: smoothed}
}

// alphaConcentration measures how tightly the alpha mass is concentrated.
// Higher score = more focused watermark (less noise spread).
// score = total_alpha_mass / (non_zero_pixels + 1)
func alphaConcentration(am *AlphaMap) float64 {
	var mass, nz float64
	for _, v := range am.Data {
		if v > 0.001 {
			mass += v
			nz++
		}
	}
	if nz < 1 {
		return 0
	}
	// Also factor in area ratio: prefer smaller bounding boxes
	areaRatio := float64(am.Width*am.Height) / float64(maxInt(am.Width, am.Height)*maxInt(am.Width, am.Height))
	return (mass / nz) * areaRatio
}

// SolveAlphaMap solves for the alpha map from black and gray seed images
// using the two-capture method.  It auto-adapts between two strategies:
//
//  1. Constant model (default) — assumes uniform background; works well for
//     clean seeds where bg is close to expected (black≈0, gray≈128).
//     Better for high-contrast text watermarks (baidu, doubao).
//
//  2. Gradient model — fits a linear gradient from the image edges to
//     compensate for brightness gradients in the seed images.
//     Better for seeds with non-uniform backgrounds (zhipu).
//
// Both models are computed and the one with higher alpha concentration
// (more watermark signal per pixel) is selected automatically.
func SolveAlphaMap(black, gray image.Image) *AlphaMap {
	b := black.Bounds()
	g := gray.Bounds()
	w, h := b.Dx(), b.Dy()
	if g.Dx() < w {
		w = g.Dx()
	}
	if g.Dy() < h {
		h = g.Dy()
	}

	// Shared noise parameters (computed from corner std, not gradient-dependent)
	bgConstB, bgConstG := calibrateBackground(black, gray, w, h)
	noiseFloorB := maxFloat(3.0, 3.0*bgConstB.lumStd)
	noiseFloorG := maxFloat(3.0, 3.0*bgConstG.lumStd)
	noiseGate := maxFloat(0.005, minFloat(0.03, (bgConstB.lumStd+bgConstG.lumStd)/200.0))

	// Strategy 1: constant bg model
	constB := &bgModel{baseR: bgConstB.baseR, baseG: bgConstB.baseG, baseB: bgConstB.baseB}
	constG := &bgModel{baseR: bgConstG.baseR, baseG: bgConstG.baseG, baseB: bgConstG.baseB}
	resultConst := computeAlphaWith(black, gray, w, h, constB, constG, noiseFloorB, noiseFloorG, noiseGate, false)

	// Strategy 2: gradient bg model
	gradB := &bgModel{
		baseR: bgConstB.baseR, baseG: bgConstB.baseG, baseB: bgConstB.baseB,
		gxR: bgConstB.gxR, gxG: bgConstB.gxG, gxB: bgConstB.gxB,
		gyR: bgConstB.gyR, gyG: bgConstB.gyG, gyB: bgConstB.gyB,
	}
	gradG := &bgModel{
		baseR: bgConstG.baseR, baseG: bgConstG.baseG, baseB: bgConstG.baseB,
		gxR: bgConstG.gxR, gxG: bgConstG.gxG, gxB: bgConstG.gxB,
		gyR: bgConstG.gyR, gyG: bgConstG.gyG, gyB: bgConstG.gyB,
	}
	resultGrad := computeAlphaWith(black, gray, w, h, gradB, gradG, noiseFloorB, noiseFloorG, noiseGate, true)

	// Pick the model with higher concentration (more signal per pixel)
	scoreConst := alphaConcentration(resultConst)
	scoreGrad := alphaConcentration(resultGrad)

	if scoreGrad > scoreConst {
		return resultGrad
	}
	return resultConst
}

// maxFloat returns the larger of two float64 values.
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// minFloat returns the smaller of two float64 values.
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// TrimAlphaMap trims transparent edges from the alpha map.
// Pixels below threshold are considered transparent and guide the
// bounding box; a 2-pixel padding is added around the result.
func TrimAlphaMap(am *AlphaMap, threshold float64) (*AlphaMap, int, int, int, int) {
	if am.Width == 0 || am.Height == 0 {
		return am, 0, 0, 0, 0
	}
	minX, minY := am.Width, am.Height
	maxX, maxY := 0, 0
	for y := 0; y < am.Height; y++ {
		for x := 0; x < am.Width; x++ {
			if am.Data[y*am.Width+x] > threshold {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if minX > maxX || minY > maxY {
		return am, 0, 0, am.Width, am.Height
	}
	pad := 2
	minX = maxInt(0, minX-pad)
	minY = maxInt(0, minY-pad)
	maxX = minInt(am.Width-1, maxX+pad)
	maxY = minInt(am.Height-1, maxY+pad)
	cw := maxX - minX + 1
	ch := maxY - minY + 1
	data := make([]float64, cw*ch)
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			data[y*cw+x] = am.Data[(minY+y)*am.Width+(minX+x)]
		}
	}
	return &AlphaMap{Width: cw, Height: ch, Data: data}, minX, minY, cw, ch
}

// TrimAlphaMapMass trims edges by keeping the smallest region that contains
// at least `fraction` of the total alpha mass.  This adapts automatically
// to both high-alpha text marks and low-alpha sparkle patterns.
// alphaMass is sum(alpha) over the full map; pass 0 to compute it here.
func TrimAlphaMapMass(am *AlphaMap, fraction float64) (*AlphaMap, int, int, int, int) {
	if am.Width == 0 || am.Height == 0 || fraction >= 1 {
		return TrimAlphaMap(am, 0)
	}

	// Compute total alpha mass
	totalMass := 0.0
	for i := range am.Data {
		totalMass += am.Data[i]
	}
	if totalMass < 1e-10 {
		return TrimAlphaMap(am, 0)
	}

	// Compute row and column marginals
	rowMass := make([]float64, am.Height)
	colMass := make([]float64, am.Width)
	for y := 0; y < am.Height; y++ {
		for x := 0; x < am.Width; x++ {
			v := am.Data[y*am.Width+x]
			rowMass[y] += v
			colMass[x] += v
		}
	}

	// Trim from top until cumulative exceeds waste margin
	trimFraction := (1 - fraction) / 2
	waste := totalMass * trimFraction

	// Top
	var cum float64
	top := 0
	for top < am.Height && cum+rowMass[top] <= waste {
		cum += rowMass[top]
		top++
	}

	// Bottom
	cum = 0
	bot := am.Height - 1
	for bot >= 0 && cum+rowMass[bot] <= waste {
		cum += rowMass[bot]
		bot--
	}

	// Left
	cum = 0
	left := 0
	for left < am.Width && cum+colMass[left] <= waste {
		cum += colMass[left]
		left++
	}

	// Right
	cum = 0
	right := am.Width - 1
	for right >= 0 && cum+colMass[right] <= waste {
		cum += colMass[right]
		right--
	}

	if left > right || top > bot {
		return TrimAlphaMap(am, 0)
	}

	pad := 2
	left = maxInt(0, left-pad)
	top = maxInt(0, top-pad)
	right = minInt(am.Width-1, right+pad)
	bot = minInt(am.Height-1, bot+pad)

	cw := right - left + 1
	ch := bot - top + 1
	data := make([]float64, cw*ch)
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			data[y*cw+x] = am.Data[(top+y)*am.Width+(left+x)]
		}
	}
	return &AlphaMap{Width: cw, Height: ch, Data: data}, left, top, cw, ch
}

// LearnResult holds the result of learning a watermark from seed images.
type LearnResult struct {
	Name               string
	AlphaMap           *AlphaMap
	NativeWidth        int
	MarginXFrac        float64
	MarginYFrac        float64
	DetectThreshold    float64
	RemoveStrategy     string
	OversubtractMargin float64
}

// LearnWatermark solves the alpha map from black+gray seed images and
// auto-derives all config parameters.  Uses mass-preserving trimming
// (keeps 99.5% of alpha energy) to adapt automatically to both bright
// text marks and faint sparkle patterns.
func LearnWatermark(black, gray image.Image, name string) *LearnResult {
	b := black.Bounds()
	imgW := b.Dx()
	imgH := b.Dy()
	alpha := SolveAlphaMap(black, gray)
	// Trim using a 0.02 threshold.  The noise gate in SolveAlphaMap
	// has already zeroed out stray noise, so this threshold cleanly
	// isolates the watermark footprint.
	trimmed, offsetX, offsetY, _, _ := TrimAlphaMap(alpha, 0.02)
	marginX := imgW - offsetX - trimmed.Width
	marginY := imgH - offsetY - trimmed.Height
	marginXFrac := float64(marginX) / float64(imgW)
	marginYFrac := float64(marginY) / float64(imgW)

	// Data-driven detection threshold: 90th percentile of non-zero alpha,
	// clamped to [0.15, 0.40].  Adapts to both faint sparkle and bright text.
	detectThreshold := estimateThreshold(trimmed)

	return &LearnResult{
		Name:               name,
		AlphaMap:           trimmed,
		NativeWidth:        imgW,
		MarginXFrac:        marginXFrac,
		MarginYFrac:        marginYFrac,
		DetectThreshold:    detectThreshold,
		RemoveStrategy:     "alpha_blend",
		OversubtractMargin: 0,
	}
}

// estimateThreshold computes a data-driven detection threshold from the
// trimmed alpha map.  Uses the 90th percentile of non-zero alpha values,
// clamped to [0.15, 0.40].
func estimateThreshold(am *AlphaMap) float64 {
	// Collect non-zero alpha values
	var vals []float64
	for _, v := range am.Data {
		if v > 0.001 {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return 0.30
	}

	// Sort descending, pick the 90th percentile.
	// Alpha maps are small (<10k pixels), so a full sort is fine.
	sort.Slice(vals, func(i, j int) bool { return vals[i] > vals[j] })
	p90Idx := int(float64(len(vals)-1) * 0.90)
	p90 := vals[p90Idx]

	if p90 < 0.15 {
		return 0.15
	}
	if p90 > 0.40 {
		return 0.40
	}
	return p90
}

// SaveWatermarkPNG saves a learned watermark as a self-contained PNG file.
// The alpha map is stored as grayscale pixels; all metadata is embedded
// in PNG tEXt chunks.
func SaveWatermarkPNG(path string, lr *LearnResult) error {
	img := image.NewGray(image.Rect(0, 0, lr.AlphaMap.Width, lr.AlphaMap.Height))
	for y := 0; y < lr.AlphaMap.Height; y++ {
		for x := 0; x < lr.AlphaMap.Width; x++ {
			v := uint8(math.Round(lr.AlphaMap.Data[y*lr.AlphaMap.Width+x] * 255))
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encode alpha map PNG: %w", err)
	}
	meta := map[string]string{
		txtName:               lr.Name,
		txtNativeWidth:        strconv.Itoa(lr.NativeWidth),
		txtMarginXFrac:        floatToStr(lr.MarginXFrac),
		txtMarginYFrac:        floatToStr(lr.MarginYFrac),
		txtDetectThreshold:    floatToStr(lr.DetectThreshold),
		txtRemoveStrategy:     lr.RemoveStrategy,
		txtOversubtractMargin: floatToStr(lr.OversubtractMargin),
	}
	pngData := injectTextChunks(buf.Bytes(), meta)
	return os.WriteFile(path, pngData, 0644)
}

// injectTextChunks inserts tEXt metadata chunks after IHDR in a PNG stream.
func injectTextChunks(pngData []byte, meta map[string]string) []byte {
	const ihdrEnd = 33
	var textChunks []byte
	for k, v := range meta {
		textChunks = append(textChunks, buildTextChunk(k, v)...)
	}
	out := make([]byte, 0, len(pngData)+len(textChunks))
	out = append(out, pngData[:ihdrEnd]...)
	out = append(out, textChunks...)
	out = append(out, pngData[ihdrEnd:]...)
	return out
}

// buildTextChunk builds a PNG tEXt chunk.
func buildTextChunk(key, value string) []byte {
	payload := []byte(key + "\x00" + value)
	crcData := append([]byte("tEXt"), payload...)
	chunk := make([]byte, 4+4+len(payload)+4)
	binary.BigEndian.PutUint32(chunk[0:4], uint32(len(payload)))
	copy(chunk[4:8], "tEXt")
	copy(chunk[8:8+len(payload)], payload)
	crc := crc32IEEE(crcData)
	binary.BigEndian.PutUint32(chunk[8+len(payload):12+len(payload)], crc)
	return chunk
}

// crc32IEEE computes CRC-32/IEEE (used by PNG).
func crc32IEEE(data []byte) uint32 {
	var crc uint32 = 0xFFFFFFFF
	for _, b := range data {
		crc ^= uint32(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xEDB88320
			} else {
				crc >>= 1
			}
		}
	}
	return crc ^ 0xFFFFFFFF
}

// LoadWatermarkPNG loads a self-contained .watermark.png file.
func LoadWatermarkPNG(path string) (*LearnResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("watermark: read %s: %w", path, err)
	}
	meta := parseTextChunks(data)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("watermark: decode alpha map PNG: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	alphaData := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			alphaData[y*w+x] = float64(r>>8) / 255.0
		}
	}
	am := &AlphaMap{Width: w, Height: h, Data: alphaData}
	lr := &LearnResult{
		Name:               meta[txtName],
		AlphaMap:           am,
		NativeWidth:        parseInt(meta[txtNativeWidth], 0),
		MarginXFrac:        parseFloat(meta[txtMarginXFrac], 0),
		MarginYFrac:        parseFloat(meta[txtMarginYFrac], 0),
		DetectThreshold:    parseFloat(meta[txtDetectThreshold], 0.30),
		RemoveStrategy:     meta[txtRemoveStrategy],
		OversubtractMargin: parseFloat(meta[txtOversubtractMargin], 0),
	}
	if lr.RemoveStrategy == "" {
		lr.RemoveStrategy = "alpha_blend"
	}
	return lr, nil
}

// parseTextChunks extracts all tEXt chunk key-value pairs from a PNG file.
func parseTextChunks(data []byte) map[string]string {
	meta := make(map[string]string)
	if len(data) < 33 {
		return meta
	}
	pos := 33
	for pos+12 <= len(data) {
		if pos+4 > len(data) {
			break
		}
		chunkLen := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		pos += 4
		if pos+4 > len(data) {
			break
		}
		chunkType := string(data[pos : pos+4])
		pos += 4
		if chunkType == "IEND" {
			break
		}
		if chunkType == "tEXt" && chunkLen > 0 && pos+chunkLen <= len(data) {
			payload := data[pos : pos+chunkLen]
			nullIdx := -1
			for i, b := range payload {
				if b == 0 {
					nullIdx = i
					break
				}
			}
			if nullIdx > 0 && nullIdx < len(payload)-1 {
				meta[string(payload[:nullIdx])] = string(payload[nullIdx+1:])
			}
		}
		pos += chunkLen + 4
	}
	return meta
}

// RegisterWatermarkPNG loads a .watermark.png file and registers it.
func RegisterWatermarkPNG(path string) error {
	lr, err := LoadWatermarkPNG(path)
	if err != nil {
		return err
	}
	RegisterLearnResult(lr)
	return nil
}

// RegisterLearnResult creates a runtime Config from a LearnResult.
func RegisterLearnResult(lr *LearnResult) {
	alphaW := lr.AlphaMap.Width
	alphaH := lr.AlphaMap.Height
	nativeW := lr.NativeWidth
	marginXFrac := lr.MarginXFrac
	marginYFrac := lr.MarginYFrac
	removeStrategy := RemoveAlphaBlend
	switch lr.RemoveStrategy {
	case "inpaint":
		removeStrategy = RemoveInpaint
	case "skip":
		removeStrategy = RemoveSkip
	}
	Register(Config{
		Type:               TypeUnknown,
		Name:               lr.Name,
		AlphaMap:           lr.AlphaMap,
		DefaultSize:        minInt(alphaW, alphaH),
		DefaultMarginX:     int(math.Round(float64(nativeW) * marginXFrac)),
		DefaultMarginY:     int(math.Round(float64(nativeW) * marginYFrac)),
		LogoColor:          [3]float64{255, 255, 255},
		DetectThreshold:    lr.DetectThreshold,
		MinSizeScale:       0.5,
		MaxSizeScale:       2.0,
		MarginRange:        16,
		RemoveStrategy:     removeStrategy,
		OversubtractMargin: lr.OversubtractMargin,
		PositionResolver: func(w, h int) []Position {
			shorter := w
			if h < shorter {
				shorter = h
			}
			scale := float64(shorter) / float64(nativeW)
			szW := int(math.Round(float64(alphaW) * scale))
			szH := int(math.Round(float64(alphaH) * scale))
			if szW < 20 || szH < 10 {
				return nil
			}
			mx := int(math.Round(float64(w) * marginXFrac))
			my := int(math.Round(float64(w) * marginYFrac))
			x := w - mx - szW
			y := h - my - szH
			if x < 0 || y < 0 || x+szW > w || y+szH > h {
				return nil
			}
			return []Position{{X: x, Y: y, W: szW, H: szH}}
		},
	})
}

// LoadWatermarkPNGsFromDir loads all .watermark.png files from a directory.
func LoadWatermarkPNGsFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var errs []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".watermark.png") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := RegisterWatermarkPNG(path); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", e.Name(), err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("watermark: load custom configs: %s", strings.Join(errs, "; "))
	}
	return nil
}

func floatToStr(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func parseFloat(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}
