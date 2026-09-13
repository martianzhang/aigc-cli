package detect

import (
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/wmremove"
)

func tryInitWMRemove() (*wmremove.Detector, error) {
	// ONNX Runtime lives in the shared models root
	lp, err := wmremove.DefaultLibPath(filepath.Join(options.ConfigDir(), "models"))
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
