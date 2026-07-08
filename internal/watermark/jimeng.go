package watermark

import "math"

func init() {
	data := make([]float64, 414*118)
	for i := 0; i < 414*118; i++ {
		data[i] = jimengAlphaRaw[i]
	}
	am := NewAlphaMap(414, 118, data)

	// Jimeng "★ 即梦AI" text watermark in bottom-right corner.
	// Alpha map is 414×118 at 2048px native width, scales with image width.
	// Parameters from remove-ai-watermarks reference project.
	const (
		nativeW     = 2048
		alphaW      = 414
		alphaH      = 118
		marginRFrac = 0.0288
		marginBFrac = 0.0288
	)

	Register(Config{
		Type:            TypeJimeng,
		Name:            "jimeng",
		AlphaMap:        am,
		DefaultSize:     96,
		DefaultMarginX:  64,
		DefaultMarginY:  64,
		LogoColor:       [3]float64{255, 255, 255},
		DetectThreshold: 0.30,
		MinSizeScale:    0.5,
		MaxSizeScale:    2.0,
		MarginRange:     16,
		PositionResolver: func(w, h int) []Position {
			scale := float64(w) / nativeW
			szW := int(math.Round(float64(alphaW) * scale))
			szH := int(math.Round(float64(alphaH) * scale))
			if szW < 20 || szH < 10 {
				return nil
			}
			marginX := int(math.Round(float64(w) * marginRFrac))
			marginY := int(math.Round(float64(h) * marginBFrac))
			x := w - marginX - szW
			y := h - marginY - szH
			if x < 0 || y < 0 || x+szW > w || y+szH > h {
				return nil
			}
			return []Position{{X: x, Y: y, W: szW, H: szH}}
		},
	})
}
