package docredact

import (
	"image"
	"image/color"
	"math"
	"sort"
	"unicode"
)

// glyphSegments splits an OCR line into per-rune boxes. PP-OCR only returns a
// line box and the recognized text, so character positions have to be
// reconstructed from the pixels: connected ink components become glyph
// candidates, then the candidates are aligned to the runes with dynamic
// programming (merging touching glyphs, splitting stray components). The
// second result reports whether the split is trustworthy; callers fall back to
// token-level boxes when it is not.
func glyphSegments(img image.Image, text string, rect image.Rectangle, base int) ([]ocrSegment, bool) {
	runes := []rune(text)
	fallback := runeSegments(base, text, rect)
	if len(runes) == 0 || rect.Dx() <= 0 || rect.Dy() <= 0 {
		return fallback, false
	}
	b := img.Bounds().Intersect(rect)
	if b.Empty() {
		return fallback, false
	}
	width := b.Dx()
	bg := lineBackground(img, b)
	profile := make([]int, width)
	mask := make([]bool, width*b.Dy())
	for x := b.Min.X; x < b.Max.X; x++ {
		col := x - b.Min.X
		for y := b.Min.Y; y < b.Max.Y; y++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if absInt(int(c.R)-int(bg.R))+absInt(int(c.G)-int(bg.G))+absInt(int(c.B)-int(bg.B)) > 55 {
				mask[(y-b.Min.Y)*width+col] = true
				profile[col]++
			}
		}
	}

	units := inkComponents(mask, width, b.Dy())
	if len(units) == 0 {
		return fallback, false
	}
	chars := make([]rune, 0, len(runes))
	for _, r := range runes {
		if !unicode.IsSpace(r) {
			chars = append(chars, r)
		}
	}
	if len(chars) == 0 {
		return fallback, false
	}
	total := 0.0
	for _, r := range chars {
		total += runeWidthWeight(r)
	}
	if total <= 0 {
		return fallback, false
	}
	expected := make([]float64, len(chars))
	for i, r := range chars {
		expected[i] = float64(width) * runeWidthWeight(r) / total
	}
	boxes, precise := alignGlyphBoxes(units, expected, profile)
	if len(boxes) != len(chars) {
		return fallback, false
	}

	out := make([]ocrSegment, 0, len(runes))
	byteOff := 0
	next := 0
	for _, r := range runes {
		size := len(string(r))
		var x0, x1 int
		if unicode.IsSpace(r) {
			x0 = 0
			if next > 0 {
				x0 = boxes[next-1].x1
			} else if len(boxes) > 0 {
				x0 = boxes[0].x0
			}
		} else {
			if next >= len(boxes) {
				return fallback, false
			}
			x0, x1 = boxes[next].x0, boxes[next].x1
			next++
		}
		out = append(out, ocrSegment{
			start: base + byteOff, end: base + byteOff + size,
			rect: image.Rect(rect.Min.X+x0, rect.Min.Y, rect.Min.X+x1, rect.Max.Y),
		})
		byteOff += size
	}
	return out, precise
}

type inkBox struct{ x0, x1 int }

// inkComponents returns the bounding boxes of 8-connected ink components,
// sorted left to right. Single-pixel specks are dropped as noise.
func inkComponents(mask []bool, width, height int) []inkBox {
	if width <= 0 || height <= 0 {
		return nil
	}
	seen := make([]bool, len(mask))
	var boxes []inkBox
	stack := make([]int, 0, 64)
	for start := range mask {
		if !mask[start] || seen[start] {
			continue
		}
		seen[start] = true
		stack = append(stack[:0], start)
		minX, maxX, minY, maxY := width, -1, height, -1
		area := 0
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			y, x := p/width, p%width
			area++
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			for dy := -1; dy <= 1; dy++ {
				ny := y + dy
				if ny < 0 || ny >= height {
					continue
				}
				for dx := -1; dx <= 1; dx++ {
					nx := x + dx
					if nx < 0 || nx >= width || (dx == 0 && dy == 0) {
						continue
					}
					np := ny*width + nx
					if mask[np] && !seen[np] {
						seen[np] = true
						stack = append(stack, np)
					}
				}
			}
		}
		if area < 2 {
			continue
		}
		boxes = append(boxes, inkBox{x0: minX, x1: maxX + 1})
	}
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].x0 != boxes[j].x0 {
			return boxes[i].x0 < boxes[j].x0
		}
		return boxes[i].x1 < boxes[j].x1
	})
	return boxes
}

// alignGlyphBoxes maps ink components to runes. Normally there are fewer
// components than runes (touching digits), so components are split. More
// components than runes means a glyph broke into disconnected strokes or the
// OCR text is off; proportional segmentation is safer there.
func alignGlyphBoxes(units []inkBox, expected []float64, profile []int) ([]inkBox, bool) {
	units = mergeTouchingUnits(units)
	if len(units) == len(expected) {
		return units, true
	}
	if len(units) > len(expected) {
		return nil, false
	}
	return splitGlyphUnits(units, expected, profile)
}

// mergeTouchingUnits joins components whose columns overlap or abut. A single
// glyph can fall into several components at low resolution (for example the
// arms of a "K"); two separate characters always leave a blank column.
func mergeTouchingUnits(units []inkBox) []inkBox {
	if len(units) < 2 {
		return units
	}
	out := make([]inkBox, 0, len(units))
	out = append(out, units[0])
	for _, u := range units[1:] {
		last := &out[len(out)-1]
		if u.x0 <= last.x1 {
			if u.x1 > last.x1 {
				last.x1 = u.x1
			}
			continue
		}
		out = append(out, u)
	}
	return out
}

// splitGlyphUnits partitions units into len(expected) cells. DP picks how many
// runes each unit covers; the split within a unit follows the expected rune
// widths and snaps to blank columns.
func splitGlyphUnits(units []inkBox, expected []float64, profile []int) ([]inkBox, bool) {
	n, m := len(units), len(expected)
	if n == 0 || n > m {
		return nil, false
	}
	prefix := make([]float64, m+1)
	for i, w := range expected {
		prefix[i+1] = prefix[i] + w
	}
	penalty := 0.35 * prefix[m] / float64(m)
	const inf = math.MaxFloat64 / 4
	dp := make([][]float64, n+1)
	from := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]float64, m+1)
		from[i] = make([]int, m+1)
		for j := range dp[i] {
			dp[i][j] = inf
		}
	}
	dp[0][0] = 0
	for i := 1; i <= n; i++ {
		uw := float64(units[i-1].x1 - units[i-1].x0)
		for j := i; j <= m-(n-i); j++ {
			for k := 1; j-k >= i-1; k++ {
				ws := prefix[j] - prefix[j-k]
				cost := dp[i-1][j-k] + math.Abs(uw-ws) + float64(k-1)*penalty
				if cost < dp[i][j] {
					dp[i][j] = cost
					from[i][j] = k
				}
				if k > 6 {
					break
				}
			}
		}
	}
	if dp[n][m] >= inf {
		return nil, false
	}
	groups := make([][]inkBox, 0, n)
	i, j := n, m
	for i > 0 {
		k := from[i][j]
		groups = append(groups, splitUnit(units[i-1], expected[j-k:j], profile))
		j -= k
		i--
	}
	out := make([]inkBox, 0, m)
	for g := len(groups) - 1; g >= 0; g-- {
		out = append(out, groups[g]...)
	}
	return out, true
}

func splitUnit(u inkBox, expected []float64, profile []int) []inkBox {
	if len(expected) <= 1 {
		return []inkBox{u}
	}
	total := 0.0
	for _, w := range expected {
		total += w
	}
	width := u.x1 - u.x0
	start := u.x0
	acc := 0.0
	out := make([]inkBox, 0, len(expected))
	for i := 0; i < len(expected)-1; i++ {
		acc += expected[i]
		target := u.x0 + int(float64(width)*acc/total)
		radius := int(float64(width) * expected[i] / total * 0.35)
		if radius < 1 {
			radius = 1
		}
		if x, ok := nearestBlankColumn(profile, target, radius); ok && x > start {
			target = x
		}
		if target <= start {
			target = start + 1
		}
		if target >= u.x1 {
			target = u.x1
		}
		out = append(out, inkBox{x0: start, x1: target})
		start = target
	}
	return append(out, inkBox{x0: start, x1: u.x1})
}

// nearestBlankColumn returns the blank column closest to want within radius.
func nearestBlankColumn(profile []int, want, radius int) (int, bool) {
	lo, hi := want-radius, want+radius
	if lo < 0 {
		lo = 0
	}
	if hi >= len(profile) {
		hi = len(profile) - 1
	}
	best, bestDist := -1, 1<<30
	for x := lo; x <= hi; x++ {
		if profile[x] != 0 {
			continue
		}
		d := absInt(x - want)
		if d < bestDist {
			best, bestDist = x, d
		}
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// lineBackground estimates the paper colour by taking the most common
// luminance bin inside the line box. Sampling a single corner is fragile once
// the box has been trimmed to the ink extents.
func lineBackground(img image.Image, b image.Rectangle) color.NRGBA {
	var hist [256]int
	var sumR, sumG, sumB [256]int
	step := 1
	if b.Dx()*b.Dy() > 40000 {
		step = 2
	}
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			lum := (int(c.R)*299 + int(c.G)*587 + int(c.B)*114) / 1000
			hist[lum]++
			sumR[lum] += int(c.R)
			sumG[lum] += int(c.G)
			sumB[lum] += int(c.B)
		}
	}
	best := 0
	for i := 1; i < 256; i++ {
		if hist[i] > hist[best] {
			best = i
		}
	}
	if hist[best] == 0 {
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	n := hist[best]
	return color.NRGBA{R: uint8(sumR[best] / n), G: uint8(sumG[best] / n), B: uint8(sumB[best] / n), A: 255}
}
