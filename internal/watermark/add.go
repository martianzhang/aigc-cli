package watermark

import (
	"fmt"
	"image"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// AddWatermark applies a visible watermark to an image.
//
// Known producers (gemini, doubao, jimeng) use the registered alpha map and
// position geometry. Unknown producers render the producer text as a watermark
// using a built-in bitmap font, placed in the bottom-right corner.
//
// Returns the watermarked image and a result descriptor.
func AddWatermark(img image.Image, producer string) (*image.RGBA, *Result, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if producer == "" {
		return nil, nil, fmt.Errorf("watermark: producer is required")
	}

	// Check if it's a known registered watermark
	if cfg, ok := findConfigByName(producer); ok {
		return addKnownWatermark(img, w, h, cfg)
	}

	// Custom text watermark: render producer text using a bitmap font
	return addTextWatermark(img, w, h, producer)
}

// addKnownWatermark overlays a registered watermark (gemini/doubao/jimeng).
func addKnownWatermark(img image.Image, w, h int, cfg Config) (*image.RGBA, *Result, error) {
	srcAlpha := cfg.AlphaMap.Data
	srcW, srcH := cfg.AlphaMap.Width, cfg.AlphaMap.Height
	logo := cfg.LogoColor

	var positions []Position
	if cfg.PositionResolver != nil {
		positions = cfg.PositionResolver(w, h)
	} else {
		// Gemini: use catalog
		entries := resolveWatermarkConfigs(w, h)
		if len(entries) == 0 {
			return nil, nil, fmt.Errorf("watermark: no position found for %s at %dx%d", cfg.Name, w, h)
		}
		// Use the first valid entry (highest priority in catalog)
		e := entries[0]
		positions = []Position{{
			X: w - e.marginX - e.logoSize,
			Y: h - e.marginY - e.logoSize,
			W: e.logoSize,
			H: e.logoSize,
		}}
	}

	if len(positions) == 0 {
		return nil, nil, fmt.Errorf("watermark: no valid position for %s at %dx%d", cfg.Name, w, h)
	}

	pos := positions[0]
	dst := cloneToRGBA(img)

	// Resize alpha map to match the position dimensions
	alpha := resizeAlpha(srcAlpha, srcW, srcH, pos.W, pos.H)

	// Apply overlay: result = alpha * logo + (1-alpha) * original
	for dy := 0; dy < pos.H; dy++ {
		for dx := 0; dx < pos.W; dx++ {
			a := alpha[dy*pos.W+dx]
			if a < 0.002 {
				continue
			}
			if a > 0.99 {
				a = 0.99
			}
			inv := 1.0 - a

			px, py := pos.X+dx, pos.Y+dy
			if px < 0 || py < 0 || px >= w || py >= h {
				continue
			}
			off := dst.PixOffset(px, py)
			r := float64(dst.Pix[off+0])
			g := float64(dst.Pix[off+1])
			b := float64(dst.Pix[off+2])

			dst.Pix[off+0] = clampByte(a*logo[0] + inv*r)
			dst.Pix[off+1] = clampByte(a*logo[1] + inv*g)
			dst.Pix[off+2] = clampByte(a*logo[2] + inv*b)
		}
	}

	sz := pos.W
	if pos.H < sz {
		sz = pos.H
	}
	return dst, &Result{
		Removed:    true,
		Name:       cfg.Name,
		Confidence: 1.0,
		Size:       sz,
		Region:     fmt.Sprintf("%d,%d,%d,%d", pos.X, pos.Y, pos.W, pos.H),
	}, nil
}

// addTextWatermark renders producer text as a watermark using a bitmap font.
// The text is placed in the bottom-right corner.
func addTextWatermark(img image.Image, w, h int, text string) (*image.RGBA, *Result, error) {
	// Estimate text dimensions from the bitmap font (Face7x13: 7px wide per char, 13px tall)
	charW := 7
	charH := 13
	textW := len(text) * charW
	textH := charH

	// Add some padding
	padX := 8
	padY := 4
	tw := textW + padX*2
	th := textH + padY*2

	// Create a temporary RGBA image for the text
	tmp := image.NewRGBA(image.Rect(0, 0, tw, th))

	// Draw the text in white
	d := &font.Drawer{
		Dst:  tmp,
		Src:  image.White,
		Face: basicfont.Face7x13,
		Dot:  fixed.P(padX, padY+charH-2), // adjust for baseline
	}
	d.DrawString(text)

	// Extract alpha map from the rendered text (white pixels = watermark)
	alpha := make([]float64, tw*th)
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			off := tmp.PixOffset(x, y)
			// Use the max RGB channel as alpha proxy
			maxC := float64(tmp.Pix[off+0])
			if float64(tmp.Pix[off+1]) > maxC {
				maxC = float64(tmp.Pix[off+1])
			}
			if float64(tmp.Pix[off+2]) > maxC {
				maxC = float64(tmp.Pix[off+2])
			}
			alpha[y*tw+x] = maxC / 255.0
		}
	}

	// Position in bottom-right corner with small margin
	marginX := maxInt(10, int(float64(w)*0.01))
	marginY := maxInt(10, int(float64(h)*0.01))
	// Scale text size relative to image width (approx 0.025 of image width for 7px font)
	fontScale := math.Max(1, math.Round(float64(w)*0.025/float64(charW)))
	finalW := int(float64(tw) * fontScale)
	finalH := int(float64(th) * fontScale)

	// Ensure the text fits
	if finalW > w-marginX {
		finalW = w - marginX
	}
	if finalH > h-marginY {
		finalH = h - marginY
	}
	if finalW < 16 || finalH < 8 {
		// Minimum size
		finalW = maxInt(16, finalW)
		finalH = maxInt(8, finalH)
	}

	px := w - marginX - finalW
	py := h - marginY - finalH

	// Resize alpha map
	alpha = resizeAlpha(alpha, tw, th, finalW, finalH)

	// Apply overlay with white logo
	dst := cloneToRGBA(img)
	logo := [3]float64{255, 255, 255}
	for dy := 0; dy < finalH; dy++ {
		for dx := 0; dx < finalW; dx++ {
			a := alpha[dy*finalW+dx]
			if a < 0.01 {
				continue
			}
			if a > 0.99 {
				a = 0.99
			}
			inv := 1.0 - a

			x, y := px+dx, py+dy
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			off := dst.PixOffset(x, y)
			r := float64(dst.Pix[off+0])
			g := float64(dst.Pix[off+1])
			b := float64(dst.Pix[off+2])

			dst.Pix[off+0] = clampByte(a*logo[0] + inv*r)
			dst.Pix[off+1] = clampByte(a*logo[1] + inv*g)
			dst.Pix[off+2] = clampByte(a*logo[2] + inv*b)
		}
	}

	return dst, &Result{
		Removed:    false,
		Name:       text,
		Confidence: 1.0,
		Size:       finalW,
		Region:     fmt.Sprintf("%d,%d,%d,%d", px, py, finalW, finalH),
	}, nil
}
