package cmd

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/martianzhang/aigc-cli/internal/forensic"
	"github.com/martianzhang/aigc-cli/internal/onnx"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/watermark"
	"github.com/martianzhang/aigc-cli/internal/wmremove"
)

var wmDetector *wmremove.Detector

// detectOnce lazily initializes the ONNX detector and caches the instance
// for reuse across detect files, avoiding repeated model loading.
var detectOnce struct {
	sync.Once
	detector *onnx.Detector
}

func detectFiles(paths []string, pathOverride string) error {
	if detectOnce.detector == nil {
		detectOnce.Do(func() {
			detectOnce.detector = tryInitONNX()
		})
	}
	aiDetector := detectOnce.detector
	if aiDetector != nil {
		defer aiDetector.Close()
	}
	if detectRemoveWM {
		if d, err := tryInitWMRemove(); err == nil {
			wmDetector = d
		}
	}

	if detectJSON {
		return detectFilesJSON(paths, pathOverride, aiDetector)
	}

	for _, path := range paths {
		if err := detectOneFile(path, pathOverride, aiDetector); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	return nil
}

func detectOneFile(path, pathOverride string, aiDetector *onnx.Detector) error {
	result, err := service.DetectImage(path)
	if err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	if pathOverride != "" {
		result.Path = pathOverride
	}

	var onnxScore float64 = -1
	var onnxModelSize string
	if aiDetector != nil {
		aiResult, err := aiDetector.DetectFile(path)
		if err == nil {
			onnxScore = aiResult.AIGenRate
			onnxModelSize = modelSizeLabel(aiDetector.ModelPath())
		}
	}

	fftScore := analyzeFFTFile(path)
	noiseScore := analyzeNoiseFile(path)
	jpegScore := analyzeJPEGFile(path)

	opts := forensic.Options{
		C2PAPresent:    result.C2PA != nil && result.C2PA.Present,
		C2PAVendor:     safeC2PAVendor(result.C2PA),
		C2PASource:     safeC2PASource(result.C2PA),
		TC260Present:   result.TC260 != nil && result.TC260.Present,
		TC260Provider:  safeTC260Provider(result.TC260),
		SynthIDPresent: result.SynthID != nil && result.SynthID.Present,
		SynthIDLikely:  result.SynthID != nil && result.SynthID.Likely,
		SynthIDSource:  safeSynthIDSource(result.SynthID),
		CameraPresent:  result.Camera != nil,
		CameraMake:     safeCameraMake(result.Camera),
		CameraModel:    safeCameraModel(result.Camera),
		ONNXScore:      onnxScore,
		ONNXModelSize:  onnxModelSize,
		FFTScore:       fftScore,
		NoiseScore:     noiseScore,
		JPEGScore:      jpegScore,
		LLMScore:       -1, // -1 = unavailable; set below if online LLM provider is configured
	}

	// Detect visible AI watermarks for AI detection signal.
	if (!opts.C2PAPresent || opts.C2PASource != "AI Generated") && !opts.TC260Present {
		f, fErr := os.Open(path)
		if fErr == nil {
			img, _, decErr := image.Decode(f)
			f.Close()
			if decErr == nil {
				if dets := watermark.DetectWatermark(img); len(dets) > 0 {
					opts.WatermarkPresent = true
					opts.WatermarkName = dets[0].Name
				}
			}
		}
	}

	// ── Online LLM assessment as additional signal ──
	dp := shared.ResolveProvider(ProviderNameDetect)
	if provider.IsOnlineProvider(dp) {
		assessment, err := provider.DescribeImage(dp, path, "Analyze this image and determine if it was AI-generated. "+
			"Look for visual artifacts, unnatural patterns, and any signs of AI generation. "+
			"Reply with only a number 0-100 where 0=certainly human, 100=certainly AI, then a brief reason.")
		if err == nil {
			opts.LLMScore = parseLLMScore(assessment)
			opts.LLMDetail = assessment
		}
	}

	fr := forensic.Analyze(opts)

	result.AIDetect = &service.AIDetectResult{
		AIGenRate: fr.AIGenRate,
		Emoji:     fr.Emoji,
		Summary:   fr.Summary,
		Details:   buildDetails(fr),
	}

	if err := service.PrintDetectResult(os.Stdout, result, true); err != nil {
		return err
	}
	if detectPreview && !detectRemoveWM {
		service.PreviewFile(path)
	}
	if detectRemoveWM {
		outPath := cleanPath(path)
		// Load learned watermarks BEFORE checking TC260/C2PA for producer,
		// so ProducerToConfig can match the provider name against registered
		// config names (e.g. "doubao" in the provider description string).
		_ = watermark.LoadWatermarkPNGsFromDir(watermarkDir())
		producer := detectWmProducer
		if producer == "" && result.TC260 != nil && result.TC260.Present {
			if cp := result.TC260.Fields[service.ContentProducerKey]; cp != "" {
				producer = watermark.ProducerToConfig(cp)
			}
			if producer == "" && result.TC260.Provider != "" {
				producer = watermark.ProducerToConfig(result.TC260.Provider)
			}
		}
		if producer == "" && result.C2PA != nil && result.C2PA.Present {
			producer = watermark.ProducerToConfig(result.C2PA.Vendor)
		}

		f, fErr := os.Open(path)
		var dets []watermark.Detection
		var decodedImg image.Image
		if fErr == nil {
			decodedImg, _, _ = image.Decode(f)
			f.Close()
			// When producer is known from metadata (C2PA/TC260), skip
			// DetectWatermark — it scans ALL registered configs and can
			// return false positives from other producers. Use the known
			// producer's PositionResolver directly instead.
			if producer == "" && decodedImg != nil {
				dets = watermark.DetectWatermark(decodedImg)
			}
		}

		removed := false
		useMIGan := wmDetector != nil && (!detectAlphaMap || detectMiGAN)

		if useMIGan {
			wmX, wmY, wmW, wmH, wmOK := resolveWMBox(producer, decodedImg, dets)
			// When producer is known but has no PositionResolver (e.g. Gemini
			// sparkle), resolveWMBox can't find the position because the line-143
			// guard skipped DetectWatermark. Retry detection now for MI-GAN.
			if !wmOK && decodedImg != nil && len(dets) == 0 && producer != "" {
				dets = watermark.DetectWatermark(decodedImg)
				wmX, wmY, wmW, wmH, wmOK = resolveWMBox("", decodedImg, dets)
			}
			// When no producer is known and auto-detection also failed, try
			// PositionResolver from any registered config as a last resort
			// before the blind bottom-right fallback (e.g. Doubao without
			// --producer, where DetectWatermark may miss on some images).
			if !wmOK && decodedImg != nil && len(dets) == 0 && producer == "" {
				b := decodedImg.Bounds()
				for _, name := range watermark.RegisteredTypes() {
					if cfg, found := watermark.FindConfig(name); found && cfg.PositionResolver != nil {
						positions := cfg.PositionResolver(b.Dx(), b.Dy())
						if len(positions) > 0 {
							p := positions[0]
							wmX, wmY, wmW, wmH, wmOK = p.X, p.Y, p.W, p.H, true
							break
						}
					}
				}
			}
			if wmOK {
				removed = runMIGan(path, decodedImg, wmX, wmY, wmW, wmH, producer)
			} else if decodedImg != nil {
				b := decodedImg.Bounds()
				regionW, regionH := 300, 80
				if regionW > b.Dx() {
					regionW = b.Dx()
				}
				if regionH > b.Dy() {
					regionH = b.Dy()
				}
				removed = runMIGan(path, decodedImg, b.Dx()-regionW, b.Dy()-regionH, regionW, regionH, producer)
			}
			if !removed {
				fmt.Fprintf(os.Stderr, "  MI-GAN removal failed. Try --alpha-map or --producer <name>.\n")
			}
		} else {
			wmOK := producer != "" || len(dets) > 0 || detectAlphaMap
			if wmOK {
				res, wmErr := watermark.RemoveFileHinted(path, outPath, producer)
				if wmErr == nil && res.Removed {
					if res.Name != "" {
						fmt.Printf("  Watermark removed (alpha-map, %s) -> %s\n", res.Name, outPath)
					} else {
						fmt.Printf("  Watermark removed (alpha-map) -> %s\n", outPath)
					}
					removed = true
				}
			}
			if !removed {
				if wmDetector == nil {
					fmt.Fprintf(os.Stderr, "  MI-GAN model not found. Use --alpha-map or install migan.onnx.\n")
				} else {
					fmt.Fprintf(os.Stderr, "  Alpha-map removal failed. Try --mi-gan or --producer <name>.\n")
				}
			}
		}
		if removed && detectPreview {
			service.PreviewFile(outPath)
		}
		if !removed {
			if err := stripMetadata(path); err == nil {
				fmt.Printf("  AI metadata removed -> %s\n", outPath)
			}
		}
	}
	if detectAddWM {
		producer := detectWmProducer
		if producer == "" {
			producer = "unknown"
		}
		outPath := strings.TrimSuffix(path, filepath.Ext(path)) + "_watermarked.png"
		res, err := watermark.AddWatermarkFile(path, outPath, producer)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
		} else {
			metaNote := ""
			fmt.Printf("  Watermark added (%s%s) -> %s\n", res.Name, metaNote, outPath)
			if detectPreview {
				service.PreviewFile(outPath)
			}
		}
	}
	return nil
}

func detectFilesJSON(paths []string, pathOverride string, aiDetector *onnx.Detector) error {
	var results []*service.DetectResult
	for _, path := range paths {
		if err := detectOneFileJSON(path, pathOverride, aiDetector, &results); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
	if len(results) == 0 {
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if len(results) == 1 {
		return enc.Encode(results[0])
	}
	return enc.Encode(results)
}

func detectOneFileJSON(path, pathOverride string, aiDetector *onnx.Detector, results *[]*service.DetectResult) error {
	result, err := service.DetectImage(path)
	if err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	if pathOverride != "" {
		result.Path = pathOverride
	}

	var onnxScore float64 = -1
	var onnxModelSize string
	if aiDetector != nil {
		aiResult, err := aiDetector.DetectFile(path)
		if err == nil {
			onnxScore = aiResult.AIGenRate
			onnxModelSize = modelSizeLabel(aiDetector.ModelPath())
		}
	}

	fftScore := analyzeFFTFile(path)
	noiseScore := analyzeNoiseFile(path)
	jpegScore := analyzeJPEGFile(path)

	opts := forensic.Options{
		C2PAPresent:    result.C2PA != nil && result.C2PA.Present,
		C2PAVendor:     safeC2PAVendor(result.C2PA),
		C2PASource:     safeC2PASource(result.C2PA),
		TC260Present:   result.TC260 != nil && result.TC260.Present,
		TC260Provider:  safeTC260Provider(result.TC260),
		SynthIDPresent: result.SynthID != nil && result.SynthID.Present,
		SynthIDLikely:  result.SynthID != nil && result.SynthID.Likely,
		SynthIDSource:  safeSynthIDSource(result.SynthID),
		CameraPresent:  result.Camera != nil,
		CameraMake:     safeCameraMake(result.Camera),
		CameraModel:    safeCameraModel(result.Camera),
		ONNXScore:      onnxScore,
		ONNXModelSize:  onnxModelSize,
		FFTScore:       fftScore,
		NoiseScore:     noiseScore,
		JPEGScore:      jpegScore,
		LLMScore:       -1,
	}
	if (!opts.C2PAPresent || opts.C2PASource != "AI Generated") && !opts.TC260Present {
		f, fErr := os.Open(path)
		if fErr == nil {
			img, _, decErr := image.Decode(f)
			f.Close()
			if decErr == nil {
				if dets := watermark.DetectWatermark(img); len(dets) > 0 {
					opts.WatermarkPresent = true
					opts.WatermarkName = dets[0].Name
				}
			}
		}
	}
	fr := forensic.Analyze(opts)

	result.AIDetect = &service.AIDetectResult{
		AIGenRate: fr.AIGenRate,
		Emoji:     fr.Emoji,
		Summary:   fr.Summary,
		Details:   buildDetails(fr),
	}

	*results = append(*results, result)
	return nil
}

func tryInitWMRemove() (*wmremove.Detector, error) {
	// ONNX Runtime lives in the shared models root
	lp, err := wmremove.DefaultLibPath(filepath.Join(configDir(), "models"))
	if err != nil {
		return nil, err
	}
	mp := wmremove.DefaultModelPath(detectModelsDir())
	if _, err := os.Stat(mp); err != nil {
		return nil, fmt.Errorf("MI-GAN model not found at %s", mp)
	}
	return wmremove.NewDetector(lp, mp)
}

// removeWatermarkDetected runs MI-GAN on a detected watermark region.

// removeWatermarkMigan forces MI-GAN with a generous bottom-right mask.

// parseLLMScore extracts a 0-1 score from the LLM's response text.
// The LLM is asked to output a number 0-100, so we look for it.
func parseLLMScore(text string) float64 {
	// Scan for "N/100" or "N out of 100" pattern
	parts := strings.Fields(text)
	for i, p := range parts {
		cleaned := strings.TrimRight(p, ".,!?%")
		if n, err := strconv.Atoi(cleaned); err == nil && n >= 0 && n <= 100 {
			// Check it's not followed by a year or other non-score number
			if i+1 < len(parts) && parts[i+1] == "/100" {
				return float64(n) / 100.0
			}
			if strings.Contains(p, "/100") || strings.Contains(p, "%") {
				return float64(n) / 100.0
			}
		}
	}
	// Fallback: keyword-based heuristic
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
	aiScore = max(0, min(1, aiScore+0.5))
	return aiScore
}

// buildDetails creates a compact breakdown of all signals.
func buildDetails(r *forensic.Result) string {
	s := ""
	for _, sig := range r.Signals {
		if s != "" {
			s += "; "
		}
		s += fmt.Sprintf("%s=%.0f%%", sig.Name, sig.Score*100)
	}
	return s
}

// runMIGan runs MI-GAN inpainting on the given region and saves the result.
func runMIGan(path string, img image.Image, x, y, w, h int, producer string) bool {
	if img == nil {
		fmt.Fprintf(os.Stderr, "  MI-GAN error: no image data\n")
		return false
	}
	pad := 50
	mx := max(0, x-pad)
	my := max(0, y-pad)
	mw := w + pad*2
	mh := h + pad*2
	b := img.Bounds()
	if mx+mw > b.Dx() {
		mw = b.Dx() - mx
	}
	if my+mh > b.Dy() {
		mh = b.Dy() - my
	}
	mask := wmremove.GenerateMask(b.Dx(), b.Dy(), mx, my, mw, mh)
	outImg, err := wmDetector.RemoveWatermark(img, mask)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  MI-GAN error: %v\n", err)
		return false
	}
	outPath := cleanPath(path)
	_ = wmremove.SavePNG(outPath, outImg)
	if producer != "" {
		fmt.Printf("  Watermark removed (mi-gan, %s) -> %s\n", producer, outPath)
	} else {
		fmt.Printf("  Watermark removed (mi-gan) -> %s\n", outPath)
	}
	return true
}

// resolveWMBox returns the watermark bounding box.
//
// Priority:
//  1. --watermark-box flag (manual override)
//  2. Known producer + PositionResolver (from C2PA/TC260 metadata)
//  3. Auto-detection (scan all configs, for unknown producers)
//  4. Fallback bottom-right region
func resolveWMBox(producer string, img image.Image, dets []watermark.Detection) (x, y, w, h int, ok bool) {
	// 1. --watermark-box flag: manual override
	if detectWatermarkBox != "" && img != nil {
		b := img.Bounds()
		return parseWatermarkBox(detectWatermarkBox, b.Dx(), b.Dy())
	}

	// 2. Known producer + PositionResolver: use expected position directly.
	//    No need to validate against auto-detection — the metadata is the source of truth.
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

	// 3. Auto-detection (unknown producer, or producer without PositionResolver)
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

// parseWatermarkBox parses "x,y,w,h" or "w,h" into coordinates.
// "w,h" places the box in the bottom-right corner with 10px margin.
// Negative x/y means distance from right/bottom edge.
// All values are relative to the provided image dimensions.
func parseWatermarkBox(s string, imgW, imgH int) (x, y, w, h int, ok bool) {
	parts := strings.Split(s, ",")
	switch len(parts) {
	case 2:
		// "w,h" — bottom-right corner placement
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
						// Negative x/y: distance from right/bottom edge
						if x < 0 {
							x = imgW + x
						}
						if y < 0 {
							y = imgH + y
						}
						// Clamp to image bounds
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
