package watermark

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// standardLumTable is the IJG-recommended luminance quantization table for quality 100.
var standardLumTable = [64]int{
	16, 11, 10, 16, 24, 40, 51, 61,
	12, 12, 14, 19, 26, 58, 60, 55,
	14, 13, 16, 24, 40, 57, 69, 56,
	14, 17, 22, 29, 51, 87, 80, 62,
	18, 22, 37, 56, 68, 109, 103, 77,
	24, 35, 55, 64, 81, 104, 113, 92,
	49, 64, 78, 87, 103, 121, 120, 101,
	72, 92, 95, 98, 112, 100, 103, 99,
}

// EstimateJPEGQuality reads a JPEG file's quantization tables and estimates
// the quality setting (1–100) used when it was encoded.  Returns 90 if the
// file is not a JPEG or the table can't be parsed.
func EstimateJPEGQuality(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 90
	}
	defer f.Close()

	br := bufio.NewReader(f)
	// SOI marker
	soi := make([]byte, 2)
	if _, err := io.ReadFull(br, soi); err != nil || soi[0] != 0xFF || soi[1] != 0xD8 {
		return 90
	}

	for {
		b, err := br.ReadByte()
		if err != nil {
			return 90
		}
		// Skip 0xFF padding
		for b == 0xFF {
			b, err = br.ReadByte()
			if err != nil {
				return 90
			}
		}
		marker := b

		// SOS — start of scan data, stop looking
		if marker == 0xDA {
			break
		}
		// RST / EOI — no segment length
		if marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}

		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(br, lenBytes); err != nil {
			return 90
		}
		segLen := int(binary.BigEndian.Uint16(lenBytes)) - 2
		if segLen <= 0 {
			continue
		}

		// DQT marker — extract the first luminance table
		if marker == 0xDB {
			segData := make([]byte, segLen)
			if _, err := io.ReadFull(br, segData); err != nil {
				return 90
			}
			if q := qualityFromDQT(segData); q > 0 {
				return q
			}
			// DQT found but couldn't estimate quality — still try other markers
			continue
		}

		// Skip other segments
		if _, err := br.Discard(segLen); err != nil {
			return 90
		}
	}
	return 90
}

// qualityFromDQT extracts the luminance quantization table from a DQT segment
// and estimates the JPEG quality using the IJG formula.
func qualityFromDQT(dqtData []byte) int {
	if len(dqtData) < 65 {
		return 0
	}
	precision := (dqtData[0] >> 4) & 0x0F

	var table [64]int
	if precision == 0 {
		for i := 0; i < 64; i++ {
			table[i] = int(dqtData[1+i])
		}
	} else {
		if len(dqtData) < 129 {
			return 0
		}
		for i := 0; i < 64; i++ {
			table[i] = int(binary.BigEndian.Uint16(dqtData[1+i*2:]))
		}
	}

	// Estimate quality from each non-zero table entry using the IJG formula:
	//   Q >= 50:  tbl[i] = clamp((std[i]*(200-2*Q)+50)/100, 1, 255)
	//   Q <  50:  tbl[i] = clamp((std[i]*5000/Q+50)/100, 1, 255)
	var qSum float64
	var qCount float64

	for i := 0; i < 64; i++ {
		t := table[i]
		s := standardLumTable[i]
		if s == 0 || t == 0 {
			continue
		}

		// Try Q >= 50 formula first
		// 100*t ≈ s*(200-2*Q)+50  →  Q ≈ (200 - (100*t-50)/s) / 2
		num := float64(100*t - 50)
		den := float64(s)
		qHigh := (200.0 - num/den) / 2.0

		if qHigh >= 50 && qHigh <= 100 {
			qSum += qHigh
			qCount++
			continue
		}

		// Try Q < 50 formula
		// 100*t ≈ s*5000/Q+50  →  Q ≈ s*5000 / (100*t-50)
		if num > 0 {
			qLow := float64(s) * 5000.0 / num
			if qLow >= 1 && qLow < 50 {
				qSum += qLow
				qCount++
			}
		}
	}

	if qCount == 0 {
		return 0
	}

	q := int(math.Round(qSum / qCount))
	if q < 10 {
		return 10
	}
	if q > 100 {
		return 100
	}
	return q
}

// AddWatermarkFile adds a visible watermark to an image file and saves the result.
// For gemini, the watermark matches the AI provider's visible mark.
// For unknown producers, the producer text is rendered as a watermark.
//
// Note: This function only adds a visible watermark for testing the removal feature.
// It does NOT inject TC260 or any other metadata — the output is a plain PNG
// with no AIGC provenance claims.
func AddWatermarkFile(inputPath, outputPath, producer string) (*Result, error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	f.Close()

	dst, res, err := AddWatermark(img, producer)
	if err != nil {
		return nil, err
	}

	if outputPath == "" {
		outputPath = strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + "_watermarked.png"
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	// Encode as PNG (visible watermark only, no metadata injection)
	if err := png.Encode(out, dst); err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	res.Region = fmt.Sprintf("%s -> %s", inputPath, outputPath)
	return res, nil
}

// ProducerToConfig maps a TC260 ContentProducer value to a registered config name.
// Only gemini is built-in; all other watermarks must be learned via learn-watermark.
func ProducerToConfig(producer string) string {
	if producer == "" {
		return ""
	}
	if _, ok := findConfigByName(producer); ok {
		return producer
	}
	lower := strings.ToLower(producer)
	if strings.Contains(lower, "google") || strings.Contains(lower, "gemini") {
		return "gemini"
	}
	for _, cfg := range registry {
		if strings.Contains(lower, cfg.Name) {
			return cfg.Name
		}
	}
	return ""
}

// DetectWatermark scans an image for registered watermark types and returns
// detections sorted by confidence (highest first).
func DetectWatermark(img image.Image) []Detection {
	var results []Detection

	for _, cfg := range registry {
		det := detectWatermark(img, cfg)
		if det != nil && det.confidence >= cfg.DetectThreshold {
			results = append(results, Detection{
				Name:       cfg.Name,
				Confidence: det.confidence,
				X:          det.x,
				Y:          det.y,
				Size:       det.size,
				W:          det.w,
				H:          det.h,
			})
		}
	}

	// Sort by confidence descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Confidence > results[i].Confidence {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// RemoveWatermark detects and removes the best-matching watermark from an image.
// Returns the cleaned image and a result descriptor.
func RemoveWatermark(img image.Image) (*image.RGBA, *Result, error) {
	return RemoveWatermarkHinted(img, "")
}

// RemoveWatermarkHinted detects and removes a watermark, preferring a specific
// config when the producer is known from TC260 metadata (e.g., "doubao", "jimeng").
func RemoveWatermarkHinted(img image.Image, producer string) (*image.RGBA, *Result, error) {
	// When the producer is known, try the hinted config FIRST.
	// For PositionResolver configs (Doubao/Jimeng), use a relaxed threshold
	// because the exact position is known — no false positive risk.
	// For Gemini, use the normal threshold to avoid weak matches.
	if producer != "" {
		if cfg, ok := findConfigByName(producer); ok {
			det := detectWatermark(img, cfg)
			passThreshold := cfg.DetectThreshold
			if cfg.PositionResolver != nil {
				passThreshold = 0.08 // relaxed for known-position text marks
			}
			if det != nil && det.confidence >= passThreshold {
				dst := removeWatermark(img, det, cfg)
				region := fmt.Sprintf("%d,%d,%d,%d", det.x, det.y, det.size, det.size)
				return dst, &Result{
					Removed:    true,
					Name:       cfg.Name,
					Confidence: det.confidence,
					Size:       det.size,
					Region:     region,
				}, nil
			}
		}
	}

	// Fall back to generic detection
	detections := DetectWatermark(img)
	if len(detections) == 0 {
		return nil, &Result{Removed: false}, nil
	}

	best := detections[0]
	cfg, ok := findConfigByName(best.Name)
	if !ok {
		return nil, nil, fmt.Errorf("watermark: unknown config %q", best.Name)
	}

	// Create a candidate from the detection. Carry the detected rectangle
	// dimensions (w/h) so removeWatermark uses the full watermark bounds
	// (isTextWM path) rather than a square det.size region — required to
	// cover the whole watermark.
	det := &candidate{
		x:          best.X,
		y:          best.Y,
		size:       best.Size,
		w:          best.W,
		h:          best.H,
		confidence: best.Confidence,
	}

	dst := removeWatermark(img, det, cfg)

	region := fmt.Sprintf("%d,%d,%d,%d", det.x, det.y, det.size, det.size)
	return dst, &Result{
		Removed:    true,
		Name:       cfg.Name,
		Confidence: best.Confidence,
		Size:       det.size,
		Region:     region,
	}, nil
}

// RemoveFile loads an image, removes watermarks, and saves the result.
func RemoveFile(inputPath, outputPath string) (*Result, error) {
	return RemoveFileHinted(inputPath, outputPath, "")
}

// RemoveFileHinted is like RemoveFile but with a producer hint from metadata.
func RemoveFileHinted(inputPath, outputPath, producer string) (*Result, error) {
	f, err := os.Open(inputPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	f.Close()

	dst, res, err := RemoveWatermarkHinted(img, producer)
	if err != nil {
		return nil, err
	}
	if !res.Removed {
		return res, nil
	}

	if outputPath == "" {
		ext := filepath.Ext(inputPath)
		base := strings.TrimSuffix(inputPath, ext)
		outputPath = base + "_clean" + ext
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	ext := strings.ToLower(filepath.Ext(outputPath))
	switch ext {
	case ".jpg", ".jpeg", ".jfif":
		q := EstimateJPEGQuality(inputPath)
		if err := jpeg.Encode(out, dst, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
	default:
		if err := png.Encode(out, dst); err != nil {
			return nil, err
		}
	}

	res.Region = fmt.Sprintf("%s -> %s", inputPath, outputPath)
	return res, nil
}
