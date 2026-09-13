// Package detect holds AIGC-detection logic that is independent of the CLI
// layer: image signal analysis and parsing/scoring helpers.
package detect

import (
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/forensic"
	"github.com/martianzhang/aigc-cli/internal/watermark"
)

// AnalyzeFFTFile returns the FFT-spectrum score for an image file, or -1 when
// the file cannot be opened or decoded.
func AnalyzeFFTFile(path string) float64 {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return -1
	}
	return forensic.AnalyzeFFT(img)
}

// AnalyzeNoiseFile returns the noise-residual score for an image file, or -1
// when the file cannot be opened or decoded.
func AnalyzeNoiseFile(path string) float64 {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return -1
	}
	return forensic.AnalyzeNoiseResidual(img)
}

// AnalyzeJPEGFile returns the JPEG double-quantization score for a file.
func AnalyzeJPEGFile(path string) float64 {
	return forensic.AnalyzeJPEGDoubleQuant(path)
}

// ParseLLMScore extracts a 0-1 score from an LLM assessment, falling back to a
// keyword heuristic when no explicit number is present.
func ParseLLMScore(text string) float64 {
	parts := strings.Fields(text)
	for i, p := range parts {
		cleaned := strings.TrimRight(p, ".,!?%")
		if n, err := strconv.Atoi(cleaned); err == nil && n >= 0 && n <= 100 {
			if i+1 < len(parts) && parts[i+1] == "/100" {
				return float64(n) / 100.0
			}
			if strings.Contains(p, "/100") || strings.Contains(p, "%") {
				return float64(n) / 100.0
			}
		}
	}
	lower := strings.ToLower(text)
	aiIndicators := []string{"ai-generated", "likely ai", "artificial", "synthetic", "deepfake", "generated"}
	humanIndicators := []string{"human-made", "real photo", "natural", "authentic", "realistic"}
	aiScore := 0.0
	for _, kw := range aiIndicators {
		if strings.Contains(lower, kw) {
			aiScore += 0.3
		}
	}
	for _, kw := range humanIndicators {
		if strings.Contains(lower, kw) {
			aiScore -= 0.3
		}
	}
	return max(0, min(1, aiScore+0.5))
}

// BuildDetails renders a compact "name=score%; ..." breakdown of all signals.
func BuildDetails(r *forensic.Result) string {
	s := ""
	for _, sig := range r.Signals {
		if s != "" {
			s += "; "
		}
		s += fmt.Sprintf("%s=%.0f%%", sig.Name, sig.Score*100)
	}
	return s
}

// ParseWatermarkBox parses "x,y,w,h" or "w,h" into coordinates relative to the
// given image dimensions. "w,h" places the box in the bottom-right corner with
// a 10px margin; a negative x/y means distance from the right/bottom edge.
func ParseWatermarkBox(s string, imgW, imgH int) (x, y, w, h int, ok bool) {
	parts := strings.Split(s, ",")
	switch len(parts) {
	case 2:
		if w, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && w > 0 {
			if h, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && h > 0 {
				return imgW - w - 10, imgH - h - 10, w, h, true
			}
		}
		fmt.Fprintf(os.Stderr, "Warning: invalid --watermark-box format %q, expected \"w,h\" (e.g. \"200,60\")\n", s)
		return 0, 0, 0, 0, false
	case 4:
		if x, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
			if y, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				if w, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil && w > 0 {
					if h, err := strconv.Atoi(strings.TrimSpace(parts[3])); err == nil && h > 0 {
						if x < 0 {
							x = imgW + x
						}
						if y < 0 {
							y = imgH + y
						}
						if x < 0 {
							x = 0
						}
						if y < 0 {
							y = 0
						}
						if x+w > imgW {
							w = imgW - x
						}
						if y+h > imgH {
							h = imgH - y
						}
						if w > 0 && h > 0 {
							return x, y, w, h, true
						}
					}
				}
			}
		}
		fmt.Fprintf(os.Stderr, "Warning: invalid --watermark-box format %q, expected \"x,y,w,h\" (e.g. \"800,900,200,60\")\n", s)
		return 0, 0, 0, 0, false
	default:
		fmt.Fprintf(os.Stderr, "Warning: invalid --watermark-box format %q, expected \"w,h\" or \"x,y,w,h\"\n", s)
		return 0, 0, 0, 0, false
	}
}

// ParseCropTarget parses a crop target: "n%" (keep ratio) or "WxH" (dimensions).
func ParseCropTarget(s string) (cropW, cropH int, keepRatio float64, err error) {
	s = strings.TrimSpace(s)

	if strings.HasSuffix(s, "%") {
		pctStr := strings.TrimSuffix(s, "%")
		pct, parseErr := strconv.ParseFloat(pctStr, 64)
		if parseErr != nil || pct <= 0 || pct > 100 {
			return 0, 0, 0, fmt.Errorf("invalid percentage %q, expected 1-100%%", s)
		}
		return 0, 0, pct / 100.0, nil
	}

	parts := strings.Split(strings.ToLower(s), "x")
	if len(parts) == 2 {
		w, wErr := strconv.Atoi(strings.TrimSpace(parts[0]))
		h, hErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if wErr == nil && hErr == nil && w > 0 && h > 0 {
			return w, h, 0, nil
		}
	}

	return 0, 0, 0, fmt.Errorf("invalid format %q, expected \"WxH\" or \"n%%\"", s)
}

// ResolveWMBox returns the watermark bounding box. Priority: manual box flag,
// known producer position, auto-detection, then not-found. boxFlag is the raw
// --watermark-box value ("" when unset).
func ResolveWMBox(boxFlag, producer string, img image.Image, dets []watermark.Detection) (x, y, w, h int, ok bool) {
	if boxFlag != "" && img != nil {
		b := img.Bounds()
		return ParseWatermarkBox(boxFlag, b.Dx(), b.Dy())
	}

	if producer != "" {
		if cfg, found := watermark.FindConfig(producer); found && cfg.PositionResolver != nil && img != nil {
			b := img.Bounds()
			positions := cfg.PositionResolver(b.Dx(), b.Dy())
			if len(positions) > 0 {
				p := positions[0]
				return p.X, p.Y, p.W, p.H, true
			}
		}
	}

	if len(dets) > 0 {
		d := dets[0]
		mw := d.W
		if mw == 0 {
			mw = d.Size
		}
		mh := d.H
		if mh == 0 {
			mh = d.Size
		}
		return d.X, d.Y, mw, mh, true
	}

	return 0, 0, 0, 0, false
}
