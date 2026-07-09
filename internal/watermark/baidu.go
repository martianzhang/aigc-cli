package watermark

import "math"

func init() {
	data := make([]float64, 187*51)
	for i := 0; i < 187*51; i++ {
		data[i] = baiduAlphaRaw[i]
	}
	am := NewAlphaMap(187, 51, data)

	// Baidu "百度AI生成" text watermark in bottom-right corner.
	// Extracted via two-capture method at 1024px native width.
	const (
		nativeW    = 1024
		alphaW     = 187
		alphaH     = 51
		marginFrac = 0.00293
	)

	Register(Config{
		Type:            TypeBaidu,
		Name:            "baidu",
		AlphaMap:        am,
		DefaultSize:     51,
		DefaultMarginX:  3,
		DefaultMarginY:  3,
		LogoColor:       [3]float64{255, 255, 255},
		DetectThreshold: 0.30,
		RemoveStrategy:  RemoveAlphaBlend,
		PositionResolver: func(w, h int) []Position {
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
			marginX := int(math.Round(float64(w) * marginFrac))
			marginY := int(math.Round(float64(w) * marginFrac))
			x := w - marginX - szW
			y := h - marginY - szH
			if x < 0 || y < 0 || x+szW > w || y+szH > h {
				return nil
			}
			return []Position{{X: x, Y: y, W: szW, H: szH}}
		},
	})
}
