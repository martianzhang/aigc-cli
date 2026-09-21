package ocr

import (
	"math"
	"slices"
	"sort"
	"testing"
)

// ----- helpers -----

// boxesFromLines extracts the bboxes in the same order as the lines.
func boxesFromLines(lines []OCRLine) [][4][2]int {
	boxes := make([][4][2]int, len(lines))
	for i, l := range lines {
		boxes[i] = l.BBox
	}
	return boxes
}

// textsForBoxes maps sorted boxes back to their source line text via BBox
// identity, so assertions read as the resulting text order.
func textsForBoxes(lines []OCRLine, boxes [][4][2]int) []string {
	textByBox := make(map[[4][2]int]string, len(lines))
	for _, l := range lines {
		textByBox[l.BBox] = l.Text
	}
	texts := make([]string, len(boxes))
	for i, b := range boxes {
		texts[i] = textByBox[b]
	}
	return texts
}

// assertBoxMultisetKept fails when sorting lost, duplicated or altered a box.
func assertBoxMultisetKept(t *testing.T, before, after [][4][2]int) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("box count changed: %d -> %d", len(before), len(after))
	}
	less := func(a, b [4][2]int) int {
		for i := range a {
			for j := range a[i] {
				if a[i][j] != b[i][j] {
					if a[i][j] < b[i][j] {
						return -1
					}
					return 1
				}
			}
		}
		return 0
	}
	sortedBefore := slices.Clone(before)
	sortedAfter := slices.Clone(after)
	sort.Slice(sortedBefore, func(i, j int) bool { return less(sortedBefore[i], sortedBefore[j]) < 0 })
	sort.Slice(sortedAfter, func(i, j int) bool { return less(sortedAfter[i], sortedAfter[j]) < 0 })
	if !slices.Equal(sortedBefore, sortedAfter) {
		t.Errorf("box set changed: before=%v after=%v", sortedBefore, sortedAfter)
	}
}

// rotateBox rotates every corner of a bbox by angle radians around the origin
// (integer-rounded, mirroring the production rotation code).
func rotateBox(b [4][2]int, angle float64) [4][2]int {
	cosA, sinA := math.Cos(angle), math.Sin(angle)
	var out [4][2]int
	for i, p := range b {
		out[i][0] = int(math.Round(float64(p[0])*cosA - float64(p[1])*sinA))
		out[i][1] = int(math.Round(float64(p[0])*sinA + float64(p[1])*cosA))
	}
	return out
}

// absInt is a test-local helper (production has maxInt/minInt only).
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ----- sortBoxesGapTreeRects -----

func TestSortBoxesGapTreeRects_ReadingOrder(t *testing.T) {
	tests := []struct {
		name  string
		lines []OCRLine
		want  []string
	}{
		{
			name:  "nil slice is left untouched",
			lines: nil,
			want:  []string{},
		},
		{
			name:  "empty slice is left untouched",
			lines: []OCRLine{},
			want:  []string{},
		},
		{
			name:  "single line is left untouched",
			lines: []OCRLine{mkOCRLine("only", 10, 10, 110, 30)},
			want:  []string{"only"},
		},
		{
			name: "already sorted lines keep their order",
			lines: []OCRLine{
				mkOCRLine("first", 0, 0, 100, 20),
				mkOCRLine("second", 0, 30, 100, 50),
				mkOCRLine("third", 0, 60, 100, 80),
			},
			want: []string{"first", "second", "third"},
		},
		{
			name: "reversed lines are sorted top to bottom",
			lines: []OCRLine{
				mkOCRLine("third", 0, 60, 100, 80),
				mkOCRLine("second", 0, 30, 100, 50),
				mkOCRLine("first", 0, 0, 100, 20),
			},
			want: []string{"first", "second", "third"},
		},
		{
			// overlap == minHeight/4 is still one visual row, so the
			// left box wins even though it starts 15px lower.
			name: "same visual row is sorted left to right",
			lines: []OCRLine{
				mkOCRLine("right", 60, 0, 110, 20),
				mkOCRLine("left", 0, 15, 50, 35),
			},
			want: []string{"left", "right"},
		},
		{
			// overlap (2px) < minHeight/4 (5px): separate rows, so the
			// higher box wins even though it sits in the right column.
			name: "small vertical overlap splits rows and the higher box wins",
			lines: []OCRLine{
				mkOCRLine("right", 60, 0, 110, 20),
				mkOCRLine("left", 0, 18, 50, 38),
			},
			want: []string{"right", "left"},
		},
		{
			// A sidebar/value grid fed in reverse: GapTree groups each
			// left/right column, so the grid is read column by column.
			name: "different left edges are read column by column",
			lines: []OCRLine{
				mkOCRLine("value-b", 200, 30, 400, 50),
				mkOCRLine("label-b", 0, 30, 100, 50),
				mkOCRLine("value-a", 200, 0, 400, 20),
				mkOCRLine("label-a", 0, 0, 100, 20),
			},
			want: []string{"label-a", "label-b", "value-a", "value-b"},
		},
		{
			name: "full-width title above two columns",
			lines: []OCRLine{
				mkOCRLine("title", 0, 0, 200, 20),
				mkOCRLine("left", 0, 30, 90, 50),
				mkOCRLine("right", 110, 30, 200, 50),
			},
			want: []string{"title", "left", "right"},
		},
		{
			name: "same column with a wider lower box stays top to bottom",
			lines: []OCRLine{
				mkOCRLine("narrow", 0, 30, 50, 50),
				mkOCRLine("wide", 0, 0, 200, 20),
			},
			want: []string{"wide", "narrow"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			boxes := boxesFromLines(tc.lines)
			before := slices.Clone(boxes)

			sortBoxesGapTreeRects(boxes)

			if got := textsForBoxes(tc.lines, boxes); !slices.Equal(got, tc.want) {
				t.Errorf("sortBoxesGapTreeRects reading order = %v, want %v", got, tc.want)
			}
			assertBoxMultisetKept(t, before, boxes)
		})
	}
}

// ----- gapTreeSort -----

func TestGapTreeSort(t *testing.T) {
	tests := []struct {
		name  string
		units []boxUnit
		want  []int // expected idx sequence
	}{
		{
			name:  "empty input",
			units: nil,
			want:  []int{},
		},
		{
			name:  "single unit",
			units: []boxUnit{{x0: 0, y0: 0, x2: 100, y2: 20, idx: 0}},
			want:  []int{0},
		},
		{
			name: "stacked lines given bottom first",
			units: []boxUnit{
				{x0: 0, y0: 30, x2: 100, y2: 50, idx: 0},
				{x0: 0, y0: 0, x2: 100, y2: 20, idx: 1},
			},
			want: []int{1, 0},
		},
		{
			name: "same row given right first",
			units: []boxUnit{
				{x0: 60, y0: 0, x2: 110, y2: 20, idx: 0},
				{x0: 0, y0: 15, x2: 50, y2: 35, idx: 1},
			},
			want: []int{1, 0},
		},
		{
			name: "rows split by a small overlap keep top first",
			units: []boxUnit{
				{x0: 60, y0: 0, x2: 110, y2: 20, idx: 0},
				{x0: 0, y0: 18, x2: 50, y2: 38, idx: 1},
			},
			want: []int{0, 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sorted := gapTreeSort(slices.Clone(tc.units))
			got := make([]int, len(sorted))
			for i, u := range sorted {
				got[i] = u.idx
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("gapTreeSort idx order = %v, want %v", got, tc.want)
			}
		})
	}
}

// ----- sortGTSlice -----

func TestSortGTSlice(t *testing.T) {
	t.Run("sorts integers ascending", func(t *testing.T) {
		s := []int{3, 1, 2}
		sortGTSlice(s, func(a, b int) bool { return a < b })
		if want := []int{1, 2, 3}; !slices.Equal(s, want) {
			t.Errorf("sortGTSlice = %v, want %v", s, want)
		}
	})

	t.Run("is stable for equal keys", func(t *testing.T) {
		type pair struct {
			key int
			tag string
		}
		s := []pair{{2, "a"}, {1, "b"}, {2, "c"}}
		sortGTSlice(s, func(a, b pair) bool { return a.key < b.key })
		want := []pair{{1, "b"}, {2, "a"}, {2, "c"}}
		if !slices.Equal(s, want) {
			t.Errorf("sortGTSlice stable = %v, want %v", s, want)
		}
	})

	t.Run("empty and single element are no-ops", func(t *testing.T) {
		var empty []int
		sortGTSlice(empty, func(a, b int) bool { return a < b })
		if len(empty) != 0 {
			t.Errorf("empty slice changed: %v", empty)
		}
		one := []int{7}
		sortGTSlice(one, func(a, b int) bool { return a < b })
		if want := []int{7}; !slices.Equal(one, want) {
			t.Errorf("single slice changed: %v", one)
		}
	})
}

// ----- estimateBoxesRotation -----

func TestEstimateBoxesRotation(t *testing.T) {
	axisAligned := [4][2]int{{0, 0}, {100, 0}, {100, 20}, {0, 20}}
	tests := []struct {
		name  string
		boxes [][4][2]int
		want  float64
		tol   float64
	}{
		{
			name:  "empty input returns zero",
			boxes: nil,
			want:  0,
			tol:   0,
		},
		{
			name:  "axis aligned box returns zero",
			boxes: [][4][2]int{axisAligned},
			want:  0,
			tol:   0,
		},
		{
			name:  "axis aligned boxes return zero",
			boxes: [][4][2]int{axisAligned, {{0, 30}, {80, 30}, {80, 50}, {0, 50}}},
			want:  0,
			tol:   0,
		},
		{
			name:  "box rotated by 0.3 rad returns that angle",
			boxes: [][4][2]int{rotateBox(axisAligned, 0.3)},
			want:  0.3,
			tol:   0.01, // integer corner rounding
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := estimateBoxesRotation(tc.boxes)
			if math.Abs(got-tc.want) > tc.tol {
				t.Errorf("estimateBoxesRotation = %f, want %f (±%f)", got, tc.want, tc.tol)
			}
		})
	}
}

// ----- normalizeBoxesRotation -----

func TestNormalizeBoxesRotation_BelowThresholdIsNoop(t *testing.T) {
	boxes := [][4][2]int{
		{{0, 0}, {100, 0}, {100, 20}, {0, 20}},
		{{0, 30}, {80, 30}, {80, 50}, {0, 50}},
	}
	before := slices.Clone(boxes)

	// 0.03 rad is below the 0.05 threshold — nothing may change.
	normalizeBoxesRotation(boxes, 0.03)

	if !slices.Equal(before, boxes) {
		t.Errorf("normalizeBoxesRotation(0.03) changed boxes: before=%v after=%v", before, boxes)
	}
}

func TestNormalizeBoxesRotation_UndoesRotation(t *testing.T) {
	const angle = 0.3
	original := [4][2]int{{0, 0}, {100, 0}, {100, 20}, {0, 20}}
	boxes := [][4][2]int{rotateBox(original, angle)}

	normalizeBoxesRotation(boxes, angle)

	topLeft, topRight := boxes[0][0], boxes[0][1]
	bottomRight, bottomLeft := boxes[0][2], boxes[0][3]

	if absInt(topRight[1]-topLeft[1]) > 1 {
		t.Errorf("top edge not horizontal: %v -> %v", topLeft, topRight)
	}
	if absInt(bottomLeft[0]-topLeft[0]) > 1 {
		t.Errorf("left edge not vertical: %v -> %v", topLeft, bottomLeft)
	}
	if absInt(bottomRight[0]-topRight[0]) > 1 {
		t.Errorf("right edge not vertical: %v -> %v", topRight, bottomRight)
	}
	if w := topRight[0] - topLeft[0]; absInt(w-100) > 2 {
		t.Errorf("width = %d, want ~100", w)
	}
	if h := bottomLeft[1] - topLeft[1]; absInt(h-20) > 1 {
		t.Errorf("height = %d, want ~20", h)
	}
}
