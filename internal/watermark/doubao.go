package watermark

import "math"

func init() {
	data := make([]float64, 335*83)
	for i := 0; i < 335*83; i++ {
		data[i] = doubaoAlphaRaw[i]
	}
	am := NewAlphaMap(335, 83, data)

	// Doubao "豆包AI生成" text watermark in bottom-right corner.
	// Alpha map is 335×83 at 2048px native (square) width.
	// For non-square images, the watermark scales with the SHORTER dimension
	// (observed on 2848×1600: watermark is ~287×70, not ~466×115 as width-scaling predicts).
	// Margins (right + bottom) are a small fraction of image width.
	const (
		nativeW     = 2048
		alphaW      = 335
		alphaH      = 83
		marginRFrac = 0.0132
		marginBFrac = 0.0166
	)

	Register(Config{
		Type:            TypeDoubao,
		Name:            "doubao",
		AlphaMap:        am,
		DefaultSize:     96,
		DefaultMarginX:  32,
		DefaultMarginY:  32,
		LogoColor:       [3]float64{255, 255, 255},
		DetectThreshold: 0.30,
		MinSizeScale:    0.5,
		MaxSizeScale:    2.0,
		MarginRange:     16,
		PositionResolver: func(w, h int) []Position {
			// Scale by shorter dimension — the watermark tracks the short side
			// so it stays proportional on non-square images.
			shorter := w
			if h < shorter {
				shorter = h
			}
			scale := float64(shorter) / nativeW
			szW := int(math.Round(float64(alphaW) * scale))
			szH := int(math.Round(float64(alphaH) * scale))
			if szW < 20 || szH < 10 {
				return nil
			}
			marginX := int(math.Round(float64(w) * marginRFrac))
			marginY := int(math.Round(float64(w) * marginBFrac))
			x := w - marginX - szW
			y := h - marginY - szH
			if x < 0 || y < 0 || x+szW > w || y+szH > h {
				return nil
			}
			return []Position{{X: x, Y: y, W: szW, H: szH}}
		},
	})
}
