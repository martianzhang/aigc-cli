package watermark

// UI badge watermarks — screenshots of AI platform web UIs where a small
// dark-background "AI Generated" label is overlaid by the frontend. Unlike
// embedded watermarks (Gemini/豆包/即梦), these have an opaque or semi-opaque
// rounded background box and cannot be removed by reverse alpha blending.
// Removal is pure progressive inpaint.

func init() {
	// ── doubao-snap: "AI 生成" ──────────────────────────────────────────
	// From Doubao webpage screenshot. Fixed CSS size 118x58, top-left corner.
	data118x58 := make([]float64, 118*58)
	for i := 0; i < 118*58; i++ {
		data118x58[i] = doubaoSnapAlphaRaw[i]
	}
	am118x58 := NewAlphaMap(118, 58, data118x58)

	Register(Config{
		Type:            TypeDoubaoSnap,
		Name:            "doubao-snap",
		AlphaMap:        am118x58,
		DefaultSize:     58,
		DefaultMarginX:  10,
		DefaultMarginY:  10,
		LogoColor:       [3]float64{255, 255, 255},
		DetectThreshold: 0.25,
		RemoveStrategy:  RemoveInpaint,
		PositionResolver: func(w, h int) []Position {
			return []Position{{
				X: 10,
				Y: 10,
				W: 118,
				H: 58,
			}}
		},
	})

	// ── baidu: "百度 AI生成" ─────────────────────────────────────
	// From Baidu webpage screenshot. Fixed CSS size 139x42, bottom-right corner.
	data139x42 := make([]float64, 139*42)
	for i := 0; i < 139*42; i++ {
		data139x42[i] = baiduSnapAlphaRaw[i]
	}
	am139x42 := NewAlphaMap(139, 42, data139x42)

	Register(Config{
		Type:            TypeBaidu,
		Name:            "baidu",
		AlphaMap:        am139x42,
		DefaultSize:     42,
		DefaultMarginX:  30,
		DefaultMarginY:  15,
		LogoColor:       [3]float64{255, 255, 255},
		DetectThreshold: 0.25,
		RemoveStrategy:  RemoveInpaint,
		PositionResolver: func(w, h int) []Position {
			marginX := 30
			marginY := 15
			x := w - marginX - 139
			y := h - marginY - 42
			if x < 0 {
				x = 0
			}
			if y < 0 {
				y = 0
			}
			return []Position{{
				X: x,
				Y: y,
				W: 139,
				H: 42,
			}}
		},
	})
}
