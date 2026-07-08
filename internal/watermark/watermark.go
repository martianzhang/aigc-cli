package watermark

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

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
	detections := DetectWatermark(img)
	if len(detections) == 0 {
		return nil, &Result{Removed: false}, nil
	}

	best := detections[0]
	// If the producer is known from TC260, prefer its config even if another
	// has slightly higher confidence (known positions beat template false positives).
	if producer != "" {
		for _, d := range detections {
			if d.Name == producer || strings.Contains(d.Name, producer) {
				best = d
				break
			}
		}
	}
	cfg, ok := findConfigByName(best.Name)
	if !ok {
		return nil, nil, fmt.Errorf("watermark: unknown config %q", best.Name)
	}

	// Create a candidate from the detection
	det := &candidate{
		x: best.X, y: best.Y, size: best.Size,
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
		if err := jpeg.Encode(out, dst, &jpeg.Options{Quality: 95}); err != nil {
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
