package forensic

import (
	"image"
	"math"

	"gonum.org/v1/gonum/dsp/fourier"
)

// AnalyzeFFT performs 2D FFT power spectrum analysis on an image and returns
// a score (0-1) indicating how AI-like the spectrum appears.
// Higher score = more likely AI-generated.
// Returns -1 if analysis fails (image too small, etc.).
func AnalyzeFFT(img image.Image) float64 {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// Need at least 64x64 for meaningful FFT
	if w < 64 || h < 64 {
		return -1
	}

	// Downsample large images to 512 max dimension for performance
	maxDim := 512
	if w > maxDim || h > maxDim {
		scale := float64(maxDim) / float64(w)
		if float64(h)*scale > float64(maxDim) {
			scale = float64(maxDim) / float64(h)
		}
		nw := int(float64(w) * scale)
		nh := int(float64(h) * scale)
		if nw < 64 {
			nw = 64
		}
		if nh < 64 {
			nh = 64
		}
		return analyzeFFTSized(img, nw, nh)
	}

	return analyzeFFTSized(img, w, h)
}

func analyzeFFTSized(img image.Image, w, h int) float64 {
	// Convert to grayscale float64, centered at 0
	pixels := make([]float64, w*h)
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	for y := 0; y < h; y++ {
		srcY := y * srcH / h
		for x := 0; x < w; x++ {
			srcX := x * srcW / w
			r, g, b, _ := img.At(srcX, srcY).RGBA()
			// Convert to grayscale, normalize to [-0.5, 0.5] (zero-centered)
			gray := (float64(r+g+b) / 3 / 65535.0) - 0.5
			pixels[y*w+x] = gray
		}
	}

	// Apply Hanning window to reduce spectral leakage
	applyHanning2D(pixels, w, h)

	// 2D FFT using row-column method
	cFFT := fourier.NewCmplxFFT(w)
	rows := make([][]complex128, h)
	for y := 0; y < h; y++ {
		row := make([]complex128, w)
		for x := 0; x < w; x++ {
			row[x] = complex(pixels[y*w+x], 0)
		}
		rows[y] = cFFT.Sequence(nil, row)
	}

	cFFT2 := fourier.NewCmplxFFT(h)
	spectrum := make([][]complex128, h)
	for y := 0; y < h; y++ {
		spectrum[y] = make([]complex128, w)
	}
	for x := 0; x < w; x++ {
		col := make([]complex128, h)
		for y := 0; y < h; y++ {
			col[y] = rows[y][x]
		}
		colC := cFFT2.Sequence(nil, col)
		for y := 0; y < h; y++ {
			spectrum[y][x] = colC[y]
		}
	}

	// Compute radial power spectrum (azimuthal average)
	halfW, halfH := w/2, h/2
	maxR := halfW
	if halfH < maxR {
		maxR = halfH
	}
	if maxR < 2 {
		return 0.5
	}

	nBins := maxR
	radialSum := make([]float64, nBins)
	radialCount := make([]int, nBins)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Shift DC to center
			sx := (x + halfW) % w
			sy := (y + halfH) % h
			c := spectrum[sy][sx]
			power := real(c)*real(c) + imag(c)*imag(c)
			mag := math.Log(1e-10 + power)
			r := int(math.Sqrt(float64((x-halfW)*(x-halfW) + (y-halfH)*(y-halfH))))
			if r < nBins {
				radialSum[r] += mag
				radialCount[r]++
			}
		}
	}

	// Normalize radial bins
	profile := make([]float64, nBins)
	for r := 0; r < nBins; r++ {
		if radialCount[r] > 0 {
			profile[r] = radialSum[r] / float64(radialCount[r])
		}
	}

	// Features from power spectrum

	// 1. High-frequency energy ratio (last 30% of bins)
	hfThreshold := nBins * 7 / 10
	var lfPower, hfPower float64
	for r := 0; r < nBins; r++ {
		if r >= hfThreshold {
			hfPower += profile[r]
		} else {
			lfPower += profile[r]
		}
	}
	hfRatio := hfPower / (hfPower + lfPower + 1e-10)

	// 2. Log-log slope (1/f² deviation)
	// Fit line to log(r) vs log(profile[r]) for r >= 3 (skip DC and nearest bins)
	var sx, sy, sxy, sx2 float64
	count := 0
	for r := 3; r < nBins; r++ {
		if profile[r] <= 0 {
			continue
		}
		lr := math.Log(float64(r))
		lp := math.Log(profile[r])
		sx += lr
		sy += lp
		sxy += lr * lp
		sx2 += lr * lr
		count++
	}
	slope := -2.0 // default (natural 1/f²)
	if count > 2 {
		n := float64(count)
		slope = (n*sxy - sx*sy) / (n*sx2 - sx*sx + 1e-10)
	}

	// 3. Convert features to AI score
	// Natural images: slope ≈ -2.0 to -2.5, HF ratio varies
	// AI images: often flatter slope (-1.0 to -1.8), higher HF energy
	// Score = weighted combination

	// Slope score: slope of -2.0 → 0.5, -1.0 → 0.8, -3.0 → 0.2
	slopeScore := 0.5 - (slope+2.0)*0.3
	if slopeScore < 0 {
		slopeScore = 0
	}
	if slopeScore > 1 {
		slopeScore = 1
	}

	// HF ratio score: higher HF → more suspicious (AI artifacts)
	// Empirical: natural photos ~20-30%, AI/GAN ~25-40%
	hfScore := (hfRatio - 0.15) / 0.35
	if hfScore < 0 {
		hfScore = 0
	}
	if hfScore > 1 {
		hfScore = 1
	}

	// Combine: slope is more reliable
	score := slopeScore*0.6 + hfScore*0.4

	// Clamp
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}

// applyHanning2D applies a 2D Hanning window to the pixel data.
func applyHanning2D(pixels []float64, w, h int) {
	for y := 0; y < h; y++ {
		wy := 0.5 - 0.5*math.Cos(2*math.Pi*float64(y)/float64(h-1))
		for x := 0; x < w; x++ {
			wx := 0.5 - 0.5*math.Cos(2*math.Pi*float64(x)/float64(w-1))
			pixels[y*w+x] *= wy * wx
		}
	}
}
