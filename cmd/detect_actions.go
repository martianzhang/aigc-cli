package cmd

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/martianzhang/aigc-cli/internal/detect"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/watermark"
)

func applyWatermarkActions(path string, result *service.DetectResult) {
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
			wmX, wmY, wmW, wmH, wmOK := detect.ResolveWMBox(detectWatermarkBox, producer, decodedImg, dets)
			// When producer is known but has no PositionResolver (e.g. Gemini
			// sparkle), resolveWMBox can't find the position because the line-143
			// guard skipped DetectWatermark. Retry detection now for MI-GAN.
			if !wmOK && decodedImg != nil && len(dets) == 0 && producer != "" {
				dets = watermark.DetectWatermark(decodedImg)
				wmX, wmY, wmW, wmH, wmOK = detect.ResolveWMBox(detectWatermarkBox, "", decodedImg, dets)
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

	// --crop-watermark: generic crop-based watermark removal
	if detectCropWM != "" {
		if err := handleCropWatermark(path); err != nil {
			fmt.Fprintf(os.Stderr, "  Crop error: %v\n", err)
		}
	}
}
