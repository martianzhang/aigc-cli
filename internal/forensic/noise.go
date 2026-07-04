package forensic

import (
	"image"
	"math"
)

// SRM 5×5 kernel from Fridrich & Kodovský 2012.
// This high-pass filter suppresses image content and amplifies steganographic
// noise / AI-generation artifacts. The residual statistics (std, kurtosis)
// differ between natural photos and AI-generated images.
var srmKernel = [5][5]float64{
	{-1, 2, -2, 2, -1},
	{2, -6, 8, -6, 2},
	{-2, 8, -12, 8, -2},
	{2, -6, 8, -6, 2},
	{-1, 2, -2, 2, -1},
}

// AnalyzeNoiseResidual runs SRM high-pass filtering on the image and returns
// a score (0-1) indicating how AI-like the noise pattern appears.
// Higher score = more likely AI-generated.
// Returns -1 if analysis fails (image too small).
func AnalyzeNoiseResidual(img image.Image) float64 {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w < 16 || h < 16 {
		return -1
	}

	// Downsample large images
	maxDim := 1024
	if w > maxDim || h > maxDim {
		scale := float64(maxDim) / float64(w)
		if float64(h)*scale > float64(maxDim) {
			scale = float64(maxDim) / float64(h)
		}
		w, h = int(float64(w)*scale), int(float64(h)*scale)
		if w < 16 {
			w = 16
		}
		if h < 16 {
			h = 16
		}
	}

	return analyzeNoiseSized(img, w, h)
}

func analyzeNoiseSized(img image.Image, w, h int) float64 {
	// Convert to grayscale
	gray := make([][]float64, h)
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	for y := 0; y < h; y++ {
		gray[y] = make([]float64, w)
		srcY := y * srcH / h
		for x := 0; x < w; x++ {
			srcX := x * srcW / w
			r, g, b, _ := img.At(srcX, srcY).RGBA()
			gray[y][x] = float64(r+g+b) / 3 / 65535.0
		}
	}

	// Apply SRM 5×5 kernel (skip 2-pixel border)
	var residuals []float64
	for y := 2; y < h-2; y++ {
		for x := 2; x < w-2; x++ {
			var sum float64
			for ky := -2; ky <= 2; ky++ {
				for kx := -2; kx <= 2; kx++ {
					sum += gray[y+ky][x+kx] * srmKernel[ky+2][kx+2]
				}
			}
			residuals = append(residuals, sum/12.0)
		}
	}

	if len(residuals) == 0 {
		return 0.5
	}

	// Compute statistics
	mean := 0.0
	for _, v := range residuals {
		mean += v
	}
	mean /= float64(len(residuals))

	variance := 0.0
	for _, v := range residuals {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(residuals))
	stdDev := math.Sqrt(variance)

	// Kurtosis (excess): E[(X-μ)⁴] / σ⁴ - 3
	var m4 float64
	for _, v := range residuals {
		d := v - mean
		m4 += d * d * d * d
	}
	m4 /= float64(len(residuals))
	kurtosis := m4/(variance*variance+1e-10) - 3.0

	// Natural photos: std ~0.02-0.08, kurtosis ~0-5
	// AI-generated: often lower std (<0.02 over-smoothed), or higher kurtosis (periodic artifacts)

	// Score based on std: too low or too high = suspicious
	stdScore := 0.5
	if stdDev < 0.015 {
		stdScore = 0.7 // over-smoothed → likely AI
	} else if stdDev < 0.03 {
		stdScore = 0.55
	} else if stdDev > 0.12 {
		stdScore = 0.6 // noisy → possibly AI artifacts
	} else if stdDev > 0.08 {
		stdScore = 0.4 // slightly high → possibly real
	} else {
		stdScore = 0.2 // natural range → likely real
	}

	// Kurtosis score: high kurtosis = periodic noise = AI artifact
	kurtScore := 0.5
	if kurtosis > 10 {
		kurtScore = 0.8 // very heavy tails → suspicious
	} else if kurtosis > 5 {
		kurtScore = 0.6
	} else if kurtosis < -1 {
		kurtScore = 0.6 // too uniform → over-smoothed
	} else {
		kurtScore = 0.3 // near-Gaussian → natural
	}

	return stdScore*0.5 + kurtScore*0.5
}
