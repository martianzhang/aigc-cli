package watermark

// UI badge watermarks — screenshots of AI platform web UIs where a small
// dark-background "AI Generated" label is overlaid by the frontend. Unlike
// embedded watermarks (Gemini/豆包/即梦), these have an opaque or semi-opaque
// rounded background box and cannot be removed by reverse alpha blending.
// All badge types use RemoveSkip (detection-only, for AIGC forensic signal).

func init() {
	// ── doubao-snap: "AI 生成" ──────────────────────────────────────────
	// From Doubao webpage screenshot. Fixed CSS size 118x58, top-left corner.
	// Detection-only (RemoveSkip), used for AIGC forensic signal.
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
		RemoveStrategy:  RemoveSkip, // detection only — screenshot badge, not alpha-blended
		PositionResolver: func(w, h int) []Position {
			positions := []Position{{
				X: 10,
				Y: 10,
				W: 118,
				H: 58,
			}}
			if w > 300 && h > 150 {
				positions = append(positions, Position{X: 0, Y: 0, W: 118, H: 58})
				positions = append(positions, Position{X: 30, Y: 30, W: 118, H: 58})
			}
			return positions
		},
	})
}
