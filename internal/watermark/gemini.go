package watermark

import (
	"encoding/binary"
	"math"
)

// watermarkEntry maps an image dimension to its watermark config.
type watermarkEntry struct {
	width, height int
	logoSize      int
	marginX       int
	marginY       int
	name          string
}

// officialGeminiSizes maps official Gemini image outputs to their watermark parameters,
// matching geminiSizeCatalog.js from the reference project.
var officialGeminiSizes = func() []watermarkEntry {
	type tierCfg struct {
		logoSize, mx, my int
	}
	// Resolution tier → watermark config mapping
	tiers := map[string]tierCfg{
		"0.5k":          {48, 32, 32},
		"1k":            {96, 64, 64},
		"2k":            {96, 64, 64},
		"4k":            {96, 64, 64},
		"2k-new-margin": {96, 192, 192},
	}

	type rawEntry struct {
		family        string
		tier          string
		width, height int
	}

	// All official Gemini image sizes from the reference catalog
	raw := []rawEntry{
		// gemini-3.x-image 0.5k
		{family: "gemini-3.x-image", tier: "0.5k", width: 512, height: 512},
		{family: "gemini-3.x-image", tier: "0.5k", width: 256, height: 1024},
		{family: "gemini-3.x-image", tier: "0.5k", width: 192, height: 1536},
		{family: "gemini-3.x-image", tier: "0.5k", width: 424, height: 632},
		{family: "gemini-3.x-image", tier: "0.5k", width: 632, height: 424},
		{family: "gemini-3.x-image", tier: "0.5k", width: 448, height: 600},
		{family: "gemini-3.x-image", tier: "0.5k", width: 1024, height: 256},
		{family: "gemini-3.x-image", tier: "0.5k", width: 600, height: 448},
		{family: "gemini-3.x-image", tier: "0.5k", width: 464, height: 576},
		{family: "gemini-3.x-image", tier: "0.5k", width: 576, height: 464},
		{family: "gemini-3.x-image", tier: "0.5k", width: 1536, height: 192},
		{family: "gemini-3.x-image", tier: "0.5k", width: 384, height: 688},
		{family: "gemini-3.x-image", tier: "0.5k", width: 688, height: 384},
		{family: "gemini-3.x-image", tier: "0.5k", width: 792, height: 168},
		// gemini-3.x-image 1k
		{family: "gemini-3.x-image", tier: "1k", width: 1024, height: 1024},
		{family: "gemini-3.x-image", tier: "1k", width: 512, height: 2048},
		{family: "gemini-3.x-image", tier: "1k", width: 384, height: 3072},
		{family: "gemini-3.x-image", tier: "1k", width: 848, height: 1264},
		{family: "gemini-3.x-image", tier: "1k", width: 1264, height: 848},
		{family: "gemini-3.x-image", tier: "1k", width: 896, height: 1200},
		{family: "gemini-3.x-image", tier: "1k", width: 2048, height: 512},
		{family: "gemini-3.x-image", tier: "1k", width: 1200, height: 896},
		{family: "gemini-3.x-image", tier: "1k", width: 928, height: 1152},
		{family: "gemini-3.x-image", tier: "1k", width: 1152, height: 928},
		{family: "gemini-3.x-image", tier: "1k", width: 3072, height: 384},
		{family: "gemini-3.x-image", tier: "1k", width: 768, height: 1376},
		{family: "gemini-3.x-image", tier: "1k", width: 1376, height: 768},
		{family: "gemini-3.x-image", tier: "1k", width: 1408, height: 768},
		{family: "gemini-3.x-image", tier: "1k", width: 1584, height: 672},
		// gemini-3.x-image 2k
		{family: "gemini-3.x-image", tier: "2k", width: 2048, height: 2048},
		{family: "gemini-3.x-image", tier: "2k", width: 1024, height: 4096},
		{family: "gemini-3.x-image", tier: "2k", width: 768, height: 6144},
		{family: "gemini-3.x-image", tier: "2k", width: 1696, height: 2528},
		{family: "gemini-3.x-image", tier: "2k", width: 2528, height: 1696},
		{family: "gemini-3.x-image", tier: "2k", width: 1792, height: 2400},
		{family: "gemini-3.x-image", tier: "2k", width: 2400, height: 1792},
		{family: "gemini-3.x-image", tier: "2k", width: 1856, height: 2304},
		{family: "gemini-3.x-image", tier: "2k", width: 2304, height: 1856},
		{family: "gemini-3.x-image", tier: "2k", width: 6144, height: 768},
		{family: "gemini-3.x-image", tier: "2k", width: 1536, height: 2752},
		{family: "gemini-3.x-image", tier: "2k", width: 2752, height: 1536},
		{family: "gemini-3.x-image", tier: "2k", width: 3168, height: 1344},
		// gemini-3.x-image 2k-new-margin
		{family: "gemini-3.x-image", tier: "2k-new-margin", width: 2816, height: 1536},
		// gemini-3.x-image 4k
		{family: "gemini-3.x-image", tier: "4k", width: 4096, height: 4096},
		{family: "gemini-3.x-image", tier: "4k", width: 2048, height: 8192},
		{family: "gemini-3.x-image", tier: "4k", width: 1536, height: 12288},
		{family: "gemini-3.x-image", tier: "4k", width: 3392, height: 5056},
		{family: "gemini-3.x-image", tier: "4k", width: 5056, height: 3392},
		{family: "gemini-3.x-image", tier: "4k", width: 3584, height: 4800},
		{family: "gemini-3.x-image", tier: "4k", width: 4800, height: 3584},
		{family: "gemini-3.x-image", tier: "4k", width: 3712, height: 4608},
		{family: "gemini-3.x-image", tier: "4k", width: 4608, height: 3712},
		{family: "gemini-3.x-image", tier: "4k", width: 12288, height: 1536},
		{family: "gemini-3.x-image", tier: "4k", width: 3072, height: 5504},
		{family: "gemini-3.x-image", tier: "4k", width: 5504, height: 3072},
		{family: "gemini-3.x-image", tier: "4k", width: 6336, height: 2688},
		// gemini-2.5-flash-image 1k
		{family: "gemini-2.5-flash-image", tier: "1k", width: 1024, height: 1024},
		{family: "gemini-2.5-flash-image", tier: "1k", width: 832, height: 1248},
		{family: "gemini-2.5-flash-image", tier: "1k", width: 1248, height: 832},
		{family: "gemini-2.5-flash-image", tier: "1k", width: 864, height: 1184},
		{family: "gemini-2.5-flash-image", tier: "1k", width: 1184, height: 864},
		{family: "gemini-2.5-flash-image", tier: "1k", width: 896, height: 1152},
		{family: "gemini-2.5-flash-image", tier: "1k", width: 1152, height: 896},
		{family: "gemini-2.5-flash-image", tier: "1k", width: 768, height: 1344},
		{family: "gemini-2.5-flash-image", tier: "1k", width: 1344, height: 768},
		{family: "gemini-2.5-flash-image", tier: "1k", width: 1536, height: 672},
	}

	// Build lookup map
	type dimKey struct{ w, h int }
	entryMap := make(map[dimKey]rawEntry, len(raw))
	for _, e := range raw {
		entryMap[dimKey{e.width, e.height}] = e
	}

	// Generate watermark entries from official sizes
	var entries []watermarkEntry
	for _, e := range raw {
		base := tiers[e.tier]
		cfg := tierCfg{base.logoSize, base.mx, base.my}

		// For gemini-3.x-image 1k, override to gemini-3.x-current 1k (48px at 32px)
		if e.family == "gemini-3.x-image" && e.tier == "1k" {
			cfg = tierCfg{48, 32, 32}
		}

		entries = append(entries, watermarkEntry{
			width:    e.width,
			height:   e.height,
			logoSize: cfg.logoSize,
			marginX:  cfg.mx,
			marginY:  cfg.my,
			name:     e.family + "-" + e.tier,
		})
	}

	return entries
}()

// resolveWatermarkConfigs returns candidate watermark positions for the given image dimensions,
// following the reference project's catalog-first + near-official-projection approach.
func resolveWatermarkConfigs(w, h int) []watermarkEntry {
	type dimKey struct{ w, h int }
	var candidates []watermarkEntry

	// 1. Exact official match
	for _, e := range officialGeminiSizes {
		if e.width == w && e.height == h {
			candidates = append(candidates, e)
		}
	}

	// 2. Near-official: closest aspect-ratio match, project config.
	// Tolerance 15% handles screenshots, re-saved, and mildly cropped images
	// whose aspect ratio drifts from the official catalog entry.
	if len(candidates) == 0 {
		targetRatio := float64(w) / float64(h)
		var bestEntry *watermarkEntry
		bestDelta := 1.0
		for i := range officialGeminiSizes {
			e := &officialGeminiSizes[i]
			entryRatio := float64(e.width) / float64(e.height)
			delta := math.Abs(targetRatio-entryRatio) / entryRatio
			if delta < bestDelta {
				bestDelta = delta
				bestEntry = e
			}
		}
		if bestEntry != nil && bestDelta < 0.15 {
			scaleX := float64(w) / float64(bestEntry.width)
			scaleY := float64(h) / float64(bestEntry.height)
			projSize := int(math.Round(float64(bestEntry.logoSize) * (scaleX + scaleY) / 2))
			if projSize < 24 {
				projSize = 24
			}
			if projSize > 192 {
				projSize = 192
			}
			projMX := int(math.Max(8, math.Round(float64(bestEntry.marginX)*scaleX)))
			projMY := int(math.Max(8, math.Round(float64(bestEntry.marginY)*scaleY)))
			// Only if position is valid
			if w-projMX-projSize >= 0 && h-projMY-projSize >= 0 {
				candidates = append(candidates, watermarkEntry{
					width: w, height: h,
					logoSize: projSize,
					marginX:  projMX,
					marginY:  projMY,
					name:     bestEntry.name + "-projected",
				})
			}
		}
	}

	// 3. Always add fallback configs. For non-official sizes (screenshots,
	// re-saved, cropped), scale the watermark size by the image's short side
	// so the fallback stays proportional. The reference sizes at native
	// resolution are 96px (2k/4k) and 48px (0.5k/1k); we scale by short/2048
	// and short/1024 respectively, clamped to [24, 192].
	short := w
	if h < short {
		short = h
	}
	scale96 := float64(short) / 2048.0
	scale48 := float64(short) / 1024.0
	fb96 := int(math.Max(24, math.Min(192, math.Round(96*scale96))))
	fb48 := int(math.Max(24, math.Min(96, math.Round(48*scale48))))
	fb36 := int(math.Max(20, math.Min(72, math.Round(36*scale96))))
	mx64 := int(math.Max(8, math.Round(64*scale96)))
	mx32 := int(math.Max(4, math.Round(32*scale48)))
	mx96 := int(math.Max(8, math.Round(96*scale96)))
	mx192 := int(math.Max(8, math.Round(192*scale96)))

	fallbacks := []watermarkEntry{
		// Gemini V2 new placement: 96px, 192px margin (known as '2k-new-margin')
		{width: w, height: h, logoSize: fb96, marginX: mx192, marginY: mx192, name: "gemini-v2"},
		// Gemini V1 standard: 96px, 64px margin
		{width: w, height: h, logoSize: fb96, marginX: mx64, marginY: mx64, name: "gemini-v1"},
		// Gemini 3.x current 1k: 48px, 32px margin
		{width: w, height: h, logoSize: fb48, marginX: mx32, marginY: mx32, name: "gemini-48"},
		// Gemini 3.x large margin: 48px, 96px margin
		{width: w, height: h, logoSize: fb48, marginX: mx96, marginY: mx96, name: "gemini-48-lg"},
		// Gemini V2 small: 36px, 96px margin
		{width: w, height: h, logoSize: fb36, marginX: mx96, marginY: mx96, name: "gemini-36"},
	}
	for _, fb := range fallbacks {
		if fb.width-fb.marginX-fb.logoSize >= 0 && fb.height-fb.marginY-fb.logoSize >= 0 {
			// Deduplicate by (logoSize, marginX, marginY)
			dup := false
			for _, c := range candidates {
				if c.logoSize == fb.logoSize && c.marginX == fb.marginX && c.marginY == fb.marginY {
					dup = true
					break
				}
			}
			if !dup {
				candidates = append(candidates, fb)
			}
		}
	}

	return candidates
}

func init() {
	if len(geminiV2AlphaRaw) < 96*96*4 {
		panic("watermark: geminiV2AlphaRaw too short")
	}
	data := make([]float64, 96*96)
	for i := 0; i < 96*96; i++ {
		bits := binary.LittleEndian.Uint32(geminiV2AlphaRaw[i*4 : i*4+4])
		data[i] = float64(math.Float32frombits(bits))
	}

	am := NewAlphaMap(96, 96, data)

	// Register Gemini watermark configs. The `resolveWatermarkConfigs()` catalog
	// handles all size/margin variations — these configs just provide the alpha map
	// and threshold. Keep entries minimal to avoid false-positive competition.
	Register(Config{
		Type:            TypeGeminiSparkle,
		Name:            "gemini",
		AlphaMap:        am,
		DefaultSize:     96,
		DefaultMarginX:  64,
		DefaultMarginY:  64,
		LogoColor:       [3]float64{255, 255, 255},
		DetectThreshold: 0.35,
		MinSizeScale:    0.5,
		MaxSizeScale:    2.0,
		MarginRange:     8,
	})
}
