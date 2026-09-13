package cmd

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/watermark"
	"github.com/martianzhang/aigc-cli/internal/wmremove"
)

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
