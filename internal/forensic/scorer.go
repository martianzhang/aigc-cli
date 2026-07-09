// Package forensic provides multi-signal fusion for AI-generated content detection.
// It combines metadata signals (C2PA, TC260, SynthID, EXIF) with pixel-level
// analysis (FFT power spectrum, noise residuals) into a single confidence score.
package forensic

import (
	"fmt"
	"math"
)

// Level represents the AI-generation confidence level with emoji.
type Level int

const (
	LevelHuman       Level = iota // 🟢 Likely human-made
	LevelLow                      // 🟡 Slightly suspicious
	LevelSuspicious               // 🟠 Possibly AI-generated
	LevelLikelyAI                 // 🔴 Likely AI-generated
	LevelConfirmedAI              // 🤖 Confirmed AI-generated
)

// SignalResult holds one signal's contribution to the final score.
type SignalResult struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`  // 0-1, higher = more AI-like
	Weight float64 `json:"weight"` // importance of this signal
	Detail string  `json:"detail,omitempty"`
}

// Result holds the fused analysis output.
type Result struct {
	AIGenRate float64        `json:"ai_gen_rate"` // 0-100%
	Level     Level          `json:"level"`
	Emoji     string         `json:"emoji"`
	Signals   []SignalResult `json:"signals"`
	Summary   string         `json:"summary"`
}

const (
	weightIronclad = 100.0 // C2PA/TC260 → absolute
	weightStrong   = 40.0  // SynthID, Camera
	weightMedium   = 20.0  // ONNX model
	weightLow      = 10.0  // FFT spectrum
)

// Analyze fuses all available signals into a single Result.
// Metadata signals come from DetectResult; pixel signals come from on-the-fly analysis.
func Analyze(opts Options) *Result {
	r := &Result{}
	var signals []SignalResult
	var totalWeight, weightedScore float64

	// 1. C2PA: ironclad if source is "AI Generated"
	if opts.C2PAPresent && opts.C2PASource == "AI Generated" {
		signals = append(signals, SignalResult{
			Name: "C2PA Content Credentials", Score: 1.0, Weight: weightIronclad,
			Detail: fmt.Sprintf("Signed by %s", opts.C2PAVendor),
		})
		// C2PA is so strong it saturates the result
		r.AIGenRate = 0.99
		r.Level = LevelConfirmedAI
		r.Emoji = "🤖"
		r.Summary = levelSummary(LevelConfirmedAI, 0.99)
		r.Signals = signals
		return r
	}

	// 2. TC260: ironclad if present (Chinese national standard)
	if opts.TC260Present {
		signals = append(signals, SignalResult{
			Name: "TC260 AIGC Label", Score: 1.0, Weight: weightIronclad,
			Detail: fmt.Sprintf("Provider: %s", opts.TC260Provider),
		})
		r.AIGenRate = 0.99
		r.Level = LevelConfirmedAI
		r.Emoji = "🤖"
		r.Summary = levelSummary(LevelConfirmedAI, 0.99)
		r.Signals = signals
		return r
	}

	// 3. Visible AI watermark: ironclad if detected (Gemini/Doubao/Jimeng/Baidu/Zhipu sparkle/text).
	// Catches re-saved images that lost C2PA/TC260 metadata but still carry the
	// visible mark — common when screenshots or re-encodes strip metadata.
	if opts.WatermarkPresent {
		wmName := "Visible AI Watermark"
		if opts.WatermarkName != "" {
			wmName = fmt.Sprintf("Visible AI Watermark (%s)", opts.WatermarkName)
		}
		signals = append(signals, SignalResult{
			Name: wmName, Score: 1.0, Weight: weightIronclad,
		})
		r.AIGenRate = 0.99
		r.Level = LevelConfirmedAI
		r.Emoji = "🤖"
		r.Summary = levelSummary(LevelConfirmedAI, 0.99)
		r.Signals = signals
		return r
	}

	// 4. SynthID inference
	if opts.SynthIDPresent {
		s := 0.8
		if opts.SynthIDLikely {
			s = 0.85
		}
		signals = append(signals, SignalResult{
			Name: "SynthID Watermark", Score: s, Weight: weightStrong,
			Detail: opts.SynthIDSource,
		})
		weightedScore += s * weightStrong
		totalWeight += weightStrong
	}

	// 4. Camera EXIF → strong human signal (reverses score)
	if opts.CameraPresent {
		signals = append(signals, SignalResult{
			Name: "Camera EXIF", Score: 0.1, Weight: weightStrong,
			Detail: fmt.Sprintf("%s %s", opts.CameraMake, opts.CameraModel),
		})
		weightedScore += 0.1 * weightStrong
		totalWeight += weightStrong
	} else {
		// No camera → weak AI signal (screenshots, AI images often lack EXIF)
		signals = append(signals, SignalResult{
			Name: "No Camera EXIF", Score: 0.55, Weight: weightLow,
		})
		weightedScore += 0.55 * weightLow
		totalWeight += weightLow
	}

	// 5. ONNX model score
	if opts.ONNXScore >= 0 {
		signals = append(signals, SignalResult{
			Name: "AI Model", Score: opts.ONNXScore, Weight: weightMedium,
			Detail: opts.ONNXModelSize + " model",
		})
		weightedScore += opts.ONNXScore * weightMedium
		totalWeight += weightMedium
	}

	// 6. FFT spectral analysis
	if opts.FFTScore >= 0 {
		signals = append(signals, SignalResult{
			Name: "FFT Spectral", Score: opts.FFTScore, Weight: weightLow,
		})
		weightedScore += opts.FFTScore * weightLow
		totalWeight += weightLow
	}

	// 7. SRM noise residual analysis
	if opts.NoiseScore >= 0 {
		signals = append(signals, SignalResult{
			Name: "Noise Residual", Score: opts.NoiseScore, Weight: weightLow,
		})
		weightedScore += opts.NoiseScore * weightLow
		totalWeight += weightLow
	}

	// 8. JPEG double quantization analysis (JPEG only)
	// JPEG-specific analysis is more relevant for compressed images since
	// FFT and noise signals are degraded by lossy compression.
	if opts.JPEGScore >= 0 {
		signals = append(signals, SignalResult{
			Name: "JPEG Analysis", Score: opts.JPEGScore, Weight: weightMedium,
		})
		weightedScore += opts.JPEGScore * weightMedium
		totalWeight += weightMedium
	}

	// Compute weighted average
	if totalWeight > 0 {
		r.AIGenRate = weightedScore / totalWeight
	} else {
		r.AIGenRate = 0.5 // neutral default
	}

	// Determine level
	r.Level = scoreToLevel(r.AIGenRate)
	r.Emoji = levelEmoji(r.Level)
	r.Signals = signals
	r.Summary = levelSummary(r.Level, r.AIGenRate)

	return r
}

// Options holds all input signals for the analyzer.
type Options struct {
	C2PAPresent      bool
	C2PAVendor       string
	C2PASource       string
	TC260Present     bool
	TC260Provider    string
	SynthIDPresent   bool
	SynthIDLikely    bool
	SynthIDSource    string
	CameraPresent    bool
	CameraMake       string
	CameraModel      string
	ONNXScore        float64 // 0-1, -1 = unavailable
	ONNXModelSize    string
	FFTScore         float64 // 0-1, -1 = unavailable
	NoiseScore       float64 // SRM noise residual score, 0-1, -1 = unavailable
	JPEGScore        float64 // JPEG double quantization, 0-1, -1 = unavailable
	WatermarkPresent bool    // visible AI watermark detected (Gemini/Doubao/Jimeng/Baidu/Zhipu)
	WatermarkName    string  // detected watermark name, e.g. "gemini", "doubao"
}

func scoreToLevel(s float64) Level {
	switch {
	case s >= 0.90:
		return LevelConfirmedAI
	case s >= 0.65:
		return LevelLikelyAI
	case s >= 0.40:
		return LevelSuspicious
	case s >= 0.20:
		return LevelLow
	default:
		return LevelHuman
	}
}

func levelEmoji(l Level) string {
	switch l {
	case LevelHuman:
		return "🟢"
	case LevelLow:
		return "🟡"
	case LevelSuspicious:
		return "🟠"
	case LevelLikelyAI:
		return "🔴"
	case LevelConfirmedAI:
		return "🤖"
	default:
		return "⚪"
	}
}

func levelSummary(l Level, rate float64) string {
	pct := math.Round(rate * 100)
	switch l {
	case LevelHuman:
		return fmt.Sprintf("%s %.0f%%  Likely human-made", levelEmoji(l), pct)
	case LevelLow:
		return fmt.Sprintf("%s %.0f%%  Slightly suspicious", levelEmoji(l), pct)
	case LevelSuspicious:
		return fmt.Sprintf("%s %.0f%%  Possibly AI-generated", levelEmoji(l), pct)
	case LevelLikelyAI:
		return fmt.Sprintf("%s %.0f%%  Likely AI-generated", levelEmoji(l), pct)
	case LevelConfirmedAI:
		return fmt.Sprintf("%s %.0f%%  Confirmed AI-generated", levelEmoji(l), pct)
	default:
		return fmt.Sprintf("%s %.0f%%", levelEmoji(l), pct)
	}
}
