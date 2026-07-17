package watermark

import (
	"math"
)

// watermarkEntry describes a candidate Gemini watermark position and size.
type watermarkEntry struct {
	logoSize int
	marginX  int
	marginY  int
	name     string
}

// knownGeminiSizes maps exact image dimensions to their watermark parameters,
// matching geminiSizeCatalog.js from the reference project. When an image
// matches a known size exactly, these positions take precedence over the
// formula-based candidates because the reference catalog may use different
// margins for the same short dimension (e.g., 2k-new-margin at 2816×1536
// uses mx=192 instead of the formula's mx=144).
var knownGeminiSizes = func() map[[2]int]watermarkEntry {
	type tierCfg struct {
		logoSize, mx, my int
	}
	tiers := map[string]tierCfg{
		"0.5k":          {48, 32, 32},
		"1k":            {96, 64, 64},
		"2k":            {96, 64, 64},
		"4k":            {96, 64, 64},
		"2k-new-margin": {96, 192, 192},
	}

	type szEntry struct {
		tier          string
		width, height int
	}

	sizes := []szEntry{
		// gemini-3.x-image 0.5k
		{tier: "0.5k", width: 512, height: 512},
		{tier: "0.5k", width: 256, height: 1024},
		{tier: "0.5k", width: 192, height: 1536},
		{tier: "0.5k", width: 424, height: 632},
		{tier: "0.5k", width: 632, height: 424},
		{tier: "0.5k", width: 448, height: 600},
		{tier: "0.5k", width: 1024, height: 256},
		{tier: "0.5k", width: 600, height: 448},
		{tier: "0.5k", width: 464, height: 576},
		{tier: "0.5k", width: 576, height: 464},
		{tier: "0.5k", width: 1536, height: 192},
		{tier: "0.5k", width: 384, height: 688},
		{tier: "0.5k", width: 688, height: 384},
		{tier: "0.5k", width: 792, height: 168},
		// gemini-3.x-image 1k
		{tier: "1k", width: 1024, height: 1024},
		{tier: "1k", width: 512, height: 2048},
		{tier: "1k", width: 384, height: 3072},
		{tier: "1k", width: 848, height: 1264},
		{tier: "1k", width: 1264, height: 848},
		{tier: "1k", width: 896, height: 1200},
		{tier: "1k", width: 2048, height: 512},
		{tier: "1k", width: 1200, height: 896},
		{tier: "1k", width: 928, height: 1152},
		{tier: "1k", width: 1152, height: 928},
		{tier: "1k", width: 3072, height: 384},
		{tier: "1k", width: 768, height: 1376},
		{tier: "1k", width: 1376, height: 768},
		{tier: "1k", width: 1408, height: 768},
		{tier: "1k", width: 1584, height: 672},
		// gemini-3.x-image 2k
		{tier: "2k", width: 2048, height: 2048},
		{tier: "2k", width: 1024, height: 4096},
		{tier: "2k", width: 768, height: 6144},
		{tier: "2k", width: 1696, height: 2528},
		{tier: "2k", width: 2528, height: 1696},
		{tier: "2k", width: 1792, height: 2400},
		{tier: "2k", width: 2400, height: 1792},
		{tier: "2k", width: 1856, height: 2304},
		{tier: "2k", width: 2304, height: 1856},
		{tier: "2k", width: 6144, height: 768},
		{tier: "2k", width: 1536, height: 2752},
		{tier: "2k", width: 2752, height: 1536},
		{tier: "2k", width: 3168, height: 1344},
		// gemini-3.x-image 2k-new-margin
		{tier: "2k-new-margin", width: 2816, height: 1536},
		// gemini-3.x-image 4k
		{tier: "4k", width: 4096, height: 4096},
		{tier: "4k", width: 2048, height: 8192},
		{tier: "4k", width: 1536, height: 12288},
		{tier: "4k", width: 3392, height: 5056},
		{tier: "4k", width: 5056, height: 3392},
		{tier: "4k", width: 3584, height: 4800},
		{tier: "4k", width: 4800, height: 3584},
		{tier: "4k", width: 3712, height: 4608},
		{tier: "4k", width: 4608, height: 3712},
		{tier: "4k", width: 12288, height: 1536},
		{tier: "4k", width: 3072, height: 5504},
		{tier: "4k", width: 5504, height: 3072},
		{tier: "4k", width: 6336, height: 2688},
		// gemini-2.5-flash-image 1k
		{tier: "1k", width: 1024, height: 1024},
		{tier: "1k", width: 832, height: 1248},
		{tier: "1k", width: 1248, height: 832},
		{tier: "1k", width: 864, height: 1184},
		{tier: "1k", width: 1184, height: 864},
		{tier: "1k", width: 896, height: 1152},
		{tier: "1k", width: 1152, height: 896},
		{tier: "1k", width: 768, height: 1344},
		{tier: "1k", width: 1344, height: 768},
		{tier: "1k", width: 1536, height: 672},
	}

	m := make(map[[2]int]watermarkEntry, len(sizes))
	for _, s := range sizes {
		cfg := tiers[s.tier]
		m[[2]int{s.width, s.height}] = watermarkEntry{
			logoSize: cfg.logoSize, marginX: cfg.mx, marginY: cfg.my,
			name: "gemini-catalog-" + s.tier,
		}
	}
	return m
}()

// resolveWatermarkConfigs returns candidate watermark positions for a Gemini
// image.  Known image sizes use exact catalog positions; unknown sizes use
// a formula that scales from a 2048px (2k) reference.
//
// Formula:
//
//	Base size = 96px at short=2048 (2k), scaled by short/2048.
//	Base margin = size × 2/3 (64/96), rounded.
//	Multiple placement variants cover different model generations.
//
// See also: https://github.com/GargantuaX/gemini-watermark-remover
func resolveWatermarkConfigs(w, h int) []watermarkEntry {
	// Check exact catalog match first — the reference catalog may use
	// different margins than the formula for the same short dimension.
	var candidates []watermarkEntry
	if entry, ok := knownGeminiSizes[[2]int{w, h}]; ok {
		if w-entry.marginX-entry.logoSize >= 0 && h-entry.marginY-entry.logoSize >= 0 {
			candidates = append(candidates, entry)
		}
	}
	if entry, ok := knownGeminiSizes[[2]int{h, w}]; ok {
		entry.name = "gemini-catalog-swapped"
		if w-entry.marginX-entry.logoSize >= 0 && h-entry.marginY-entry.logoSize >= 0 {
			// Dedup with the non-swapped entry
			dup := false
			for _, c := range candidates {
				if c.logoSize == entry.logoSize && c.marginX == entry.marginX && c.marginY == entry.marginY {
					dup = true
					break
				}
			}
			if !dup {
				candidates = append(candidates, entry)
			}
		}
	}

	short := w
	if h < short {
		short = h
	}

	// Scale watermark size relative to a 2048px (2k) reference.
	scale96 := float64(short) / 2048.0
	scale48 := float64(short) / 1024.0

	// Size variants
	fb96 := int(math.Max(24, math.Min(192, math.Round(96*scale96))))
	fb48 := int(math.Max(24, math.Min(96, math.Round(48*scale48))))
	fb36 := int(math.Max(20, math.Min(72, math.Round(36*scale96))))

	// Margin variants — margin = size × 2/3 ≈ 64/96
	mx64 := int(math.Max(8, math.Round(64*scale96)))
	mx32 := int(math.Max(4, math.Round(32*scale48)))
	mx96 := int(math.Max(8, math.Round(96*scale96)))
	mx192 := int(math.Max(8, math.Round(192*scale96)))

	for _, fb := range []watermarkEntry{
		{logoSize: fb96, marginX: mx192, marginY: mx192, name: "gemini-v2"},  // new models, wide margin
		{logoSize: fb96, marginX: mx64, marginY: mx64, name: "gemini-v1"},    // standard placement
		{logoSize: fb48, marginX: mx32, marginY: mx32, name: "gemini-48"},    // small, tight
		{logoSize: fb48, marginX: mx96, marginY: mx96, name: "gemini-48-lg"}, // small, wide
		{logoSize: fb36, marginX: mx96, marginY: mx96, name: "gemini-36"},    // tiny, wide
	} {
		if w-fb.marginX-fb.logoSize >= 0 && h-fb.marginY-fb.logoSize >= 0 {
			// Dedup by (logoSize, marginX, marginY)
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
