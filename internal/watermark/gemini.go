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

// resolveWatermarkConfigs returns candidate watermark positions for a Gemini
// image of the given dimensions.  Positions are derived from a simple formula
// (not an exact catalog), so any Gemini model or image size is supported
// without code changes.
//
// Formula:
//
//	Base size = 96px at short=2048 (2k), scaled by short/2048.
//	Base margin = size × 2/3 (64/96), rounded.
//	Multiple placement variants cover different model generations.
//
// See also: https://github.com/GargantuaX/gemini-watermark-remover
func resolveWatermarkConfigs(w, h int) []watermarkEntry {
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

	// Return all viable placement variants; the caller picks the best
	// match via NCC scoring.
	var candidates []watermarkEntry
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

// Built-in Gemini registration removed. Use --learn-watermark gemini to
// generate gemini.watermark.png from seed images (scripts/assets/gemini.*).
