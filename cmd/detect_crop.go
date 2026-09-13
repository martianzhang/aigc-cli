package cmd

import (
	"fmt"
	"image"
	imagejpeg "image/jpeg"
	imagepng "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/detect"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/watermark"
)

func handleCropWatermark(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	b := img.Bounds()
	imgW, imgH := b.Dx(), b.Dy()

	var bounds watermark.CropBounds

	if detectCropWM == "" || detectCropWM == "auto" {
		regions := watermark.DetectWatermarkRegions(img)
		if len(regions) > 0 {
			bounds = watermark.ComputeCropBounds(imgW, imgH, regions)
			if !bounds.Valid {
				fmt.Printf("  Auto detection failed: %s\n", bounds.Reason)
				fmt.Printf("  Falling back to default 90%% crop (trimming 5%% from each side)\n")
				marginRatio := 0.05
				marginX := int(float64(imgW) * marginRatio)
				marginY := int(float64(imgH) * marginRatio)
				bounds = watermark.CropBounds{
					X: marginX, Y: marginY,
					W: imgW - marginX*2, H: imgH - marginY*2,
					Valid: true,
				}
			}
		} else {
			marginRatio := 0.05
			marginX := int(float64(imgW) * marginRatio)
			marginY := int(float64(imgH) * marginRatio)
			bounds = watermark.CropBounds{
				X: marginX, Y: marginY,
				W: imgW - marginX*2, H: imgH - marginY*2,
				Valid: true,
			}
			fmt.Printf("  No watermark detected, applying default margin: %.0f%%\n", marginRatio*100)
		}
	} else {
		targetW, targetH, keepRatio, parseErr := detect.ParseCropTarget(detectCropWM)
		if parseErr != nil {
			return parseErr
		}

		if keepRatio > 0 {
			newW := int(float64(imgW) * keepRatio)
			newH := int(float64(imgH) * keepRatio)
			x := (imgW - newW) / 2
			y := (imgH - newH) / 2
			bounds = watermark.CropBounds{X: x, Y: y, W: newW, H: newH, Valid: true}
		} else {
			x := (imgW - targetW) / 2
			y := (imgH - targetH) / 2
			bounds = watermark.CropBounds{X: x, Y: y, W: targetW, H: targetH, Valid: true}
		}
	}

	if bounds.W < 100 || bounds.H < 100 {
		return fmt.Errorf("crop area too small (%dx%d), minimum is 100x100", bounds.W, bounds.H)
	}

	cropped := image.NewRGBA(image.Rect(0, 0, bounds.W, bounds.H))
	for y := 0; y < bounds.H; y++ {
		for x := 0; x < bounds.W; x++ {
			cropped.Set(x, y, img.At(bounds.X+x, bounds.Y+y))
		}
	}

	outPath := cleanPath(path)
	ext := strings.ToLower(filepath.Ext(outPath))
	switch ext {
	case ".jpg", ".jpeg", ".jfif":
		q := watermark.EstimateJPEGQuality(path)
		out, oErr := os.Create(outPath)
		if oErr != nil {
			return oErr
		}
		defer out.Close()
		if err := imagejpeg.Encode(out, cropped, &imagejpeg.Options{Quality: q}); err != nil {
			return err
		}
	default:
		out, oErr := os.Create(outPath)
		if oErr != nil {
			return oErr
		}
		defer out.Close()
		if err := imagepng.Encode(out, cropped); err != nil {
			return err
		}
	}

	fmt.Printf("  Cropped: %dx%d -> %dx%d -> %s\n", imgW, imgH, bounds.W, bounds.H, outPath)
	if detectPreview {
		service.PreviewFile(outPath)
	}
	return nil
}
