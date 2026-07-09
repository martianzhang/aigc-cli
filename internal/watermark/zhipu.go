package watermark

import "math"

func init() {
	data := make([]float64, 234*60)
	for i := 0; i < 234*60; i++ {
		data[i] = zhipuAlphaRaw[i]
	}
	am := NewAlphaMap(234, 60, data)

	// Zhipu Qingyan (智谱清言 / ChatGLM) "智谱清言" text watermark in bottom-right corner.
	// Extracted via two-capture method at 1024px native width.
	// Watermark scales with the SHORTER dimension (matches doubao/baidu behavior).
	const (
		nativeW     = 1024
		alphaW      = 234
		alphaH      = 60
		marginRFrac = 0.0126953125 // ≈13/1024
		marginBFrac = 0.0078125    // ≈8/1024
	)

	Register(Config{
		Type:            TypeZhipu,
		Name:            "zhipu",
		AlphaMap:        am,
		DefaultSize:     60,
		DefaultMarginX:  13,
		DefaultMarginY:  8,
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
