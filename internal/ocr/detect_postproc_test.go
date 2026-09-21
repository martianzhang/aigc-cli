package ocr

import (
	"math"
	"testing"
)

// Tests for the pure helpers in detect_postproc.go (no ONNX/tensor dependency).
// NOTE: iouRect/boxToRect operate on the package-local rect type
// (rect{x0, y0, x1, y1 int}), not on image.Rectangle.

// ----- ocrPoint -----

func TestOCRPoint_Fields(t *testing.T) {
	positional := ocrPoint{3, 7}
	named := ocrPoint{x: 3, y: 7}

	if positional != named {
		t.Errorf("ocrPoint{3,7} != ocrPoint{x:3,y:7}")
	}
	if positional.x != 3 || positional.y != 7 {
		t.Errorf("ocrPoint{3,7} = {%d,%d}, want {3,7}", positional.x, positional.y)
	}
	if (ocrPoint{0, 0}) != (ocrPoint{}) {
		t.Error("zero-value ocrPoint should equal ocrPoint{0,0}")
	}
}

// ----- unclipBox -----

func TestUnclipBox_Expand(t *testing.T) {
	tests := []struct {
		name  string
		box   [4][2]int
		ratio float64
		want  []ocrPoint
	}{
		{
			name:  "square expansion distance int(area*ratio/perimeter)",
			box:   [4][2]int{{0, 0}, {10, 0}, {10, 10}, {0, 10}},
			ratio: 1.0, // w=h=10, area=100, perimeter=40, distance=int(2.5)=2
			want:  []ocrPoint{{-2, -2}, {12, -2}, {12, 12}, {-2, 12}},
		},
		{
			name:  "rectangle with detPostProcess unclipRatio 1.6",
			box:   [4][2]int{{10, 20}, {30, 20}, {30, 50}, {10, 50}},
			ratio: 1.6, // w=20, h=30, area=600, perimeter=100, distance=int(9.6)=9
			want:  []ocrPoint{{1, 11}, {39, 11}, {39, 59}, {1, 59}},
		},
		{
			name:  "zero ratio leaves box unchanged",
			box:   [4][2]int{{5, 5}, {15, 5}, {15, 25}, {5, 25}},
			ratio: 0,
			want:  []ocrPoint{{5, 5}, {15, 5}, {15, 25}, {5, 25}},
		},
		{
			name:  "unordered input points use same bounding rect",
			box:   [4][2]int{{30, 50}, {10, 20}, {30, 20}, {10, 50}},
			ratio: 1.6,
			want:  []ocrPoint{{1, 11}, {39, 11}, {39, 59}, {1, 59}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unclipBox(tc.box, tc.ratio)
			if !ocrPointsEqual(got, tc.want) {
				t.Errorf("unclipBox(%v, %v) = %v, want %v", tc.box, tc.ratio, got, tc.want)
			}
		})
	}
}

func TestUnclipBox_Degenerate(t *testing.T) {
	tests := []struct {
		name string
		box  [4][2]int
	}{
		{"zero width", [4][2]int{{5, 0}, {5, 0}, {5, 10}, {5, 10}}},
		{"zero height", [4][2]int{{0, 5}, {10, 5}, {10, 5}, {0, 5}}},
		{"single point", [4][2]int{{3, 3}, {3, 3}, {3, 3}, {3, 3}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := unclipBox(tc.box, 1.6); got != nil {
				t.Errorf("unclipBox(%v, 1.6) = %v, want nil", tc.box, got)
			}
		})
	}
}

// ----- minAreaRect -----

func TestMinAreaRect_Table(t *testing.T) {
	tests := []struct {
		name   string
		points []ocrPoint
		want   *[4][2]int
	}{
		{"nil points", nil, nil},
		{"single point", []ocrPoint{{3, 4}}, nil},
		{"two points below minimum", []ocrPoint{{0, 0}, {10, 10}}, nil},
		{
			name:   "three points axis-aligned",
			points: []ocrPoint{{0, 0}, {10, 0}, {5, 8}},
			want:   &[4][2]int{{0, 0}, {10, 0}, {10, 8}, {0, 8}},
		},
		{
			name:   "four unordered corners",
			points: []ocrPoint{{10, 10}, {0, 10}, {0, 0}, {10, 0}},
			want:   &[4][2]int{{0, 0}, {10, 0}, {10, 10}, {0, 10}},
		},
		{
			name:   "negative coordinates",
			points: []ocrPoint{{-5, -5}, {-1, -2}, {-3, -4}},
			want:   &[4][2]int{{-5, -5}, {-1, -5}, {-1, -2}, {-5, -2}},
		},
		{
			name:   "collinear points collapse to zero height",
			points: []ocrPoint{{0, 3}, {5, 3}, {9, 3}},
			want:   &[4][2]int{{0, 3}, {9, 3}, {9, 3}, {0, 3}},
		},
		{
			name:   "duplicate points collapse to single point rect",
			points: []ocrPoint{{7, 7}, {7, 7}, {7, 7}},
			want:   &[4][2]int{{7, 7}, {7, 7}, {7, 7}, {7, 7}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := minAreaRect(tc.points)
			if tc.want == nil {
				if got != nil {
					t.Errorf("minAreaRect(%v) = %v, want nil", tc.points, *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("minAreaRect(%v) = nil, want %v", tc.points, *tc.want)
			}
			if *got != *tc.want {
				t.Errorf("minAreaRect(%v) = %v, want %v", tc.points, *got, *tc.want)
			}
		})
	}
}

// ----- nms -----

func TestNMS_Table(t *testing.T) {
	// IoU exactly 0.5: two 3x3 boxes, intersection 3x2=6, union 12.
	halfA := [4][2]int{{0, 0}, {3, 0}, {3, 3}, {0, 3}}
	halfB := [4][2]int{{0, 1}, {3, 1}, {3, 4}, {0, 4}}
	// IoU 0.64: 10x10 box containing an 8x8 box (64/100).
	bigA := [4][2]int{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	inner := [4][2]int{{1, 1}, {9, 1}, {9, 9}, {1, 9}}
	far := [4][2]int{{100, 100}, {110, 100}, {110, 110}, {100, 110}}

	tests := []struct {
		name      string
		boxes     [][4][2]int
		scores    []float32
		threshold float32
		want      []int
	}{
		{"nil boxes returns nil", nil, nil, 0.5, nil},
		{"empty boxes returns nil", [][4][2]int{}, []float32{}, 0.5, nil},
		{
			name:      "non-overlapping boxes all kept in score order",
			boxes:     [][4][2]int{bigA, far},
			scores:    []float32{0.9, 0.8},
			threshold: 0.5,
			want:      []int{0, 1},
		},
		{
			name:      "high threshold keeps overlapping boxes",
			boxes:     [][4][2]int{bigA, inner},
			scores:    []float32{0.9, 0.8},
			threshold: 0.8, // IoU 0.64 <= 0.8 → both survive
			want:      []int{0, 1},
		},
		{
			name:      "low threshold removes overlapping box",
			boxes:     [][4][2]int{bigA, inner},
			scores:    []float32{0.9, 0.8},
			threshold: 0.5, // IoU 0.64 > 0.5 → lower-scored box dropped
			want:      []int{0},
		},
		{
			name:      "identical boxes keep only highest score",
			boxes:     [][4][2]int{bigA, bigA},
			scores:    []float32{0.9, 0.5},
			threshold: 0.5, // IoU 1.0 > 0.5
			want:      []int{0},
		},
		{
			name:      "threshold equal to IoU keeps both (<= comparison)",
			boxes:     [][4][2]int{halfA, halfB},
			scores:    []float32{0.9, 0.8},
			threshold: 0.5, // IoU exactly 0.5
			want:      []int{0, 1},
		},
		{
			name:      "threshold just below IoU suppresses",
			boxes:     [][4][2]int{halfA, halfB},
			scores:    []float32{0.9, 0.8},
			threshold: 0.49, // 0.5 > 0.49
			want:      []int{0},
		},
		{
			name:      "sorted by score not by index",
			boxes:     [][4][2]int{inner, bigA, far},
			scores:    []float32{0.3, 0.9, 0.7},
			threshold: 0.5,
			want:      []int{1, 2}, // bigA (idx 1) suppresses inner (idx 0)
		},
		{
			name:      "suppression chain skips middle box only",
			boxes:     [][4][2]int{bigA, inner, far},
			scores:    []float32{0.9, 0.8, 0.7},
			threshold: 0.5,
			want:      []int{0, 2},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nms(tc.boxes, tc.scores, tc.threshold)
			if !equalIntSlices(got, tc.want) {
				t.Errorf("nms(th=%v) = %v, want %v", tc.threshold, got, tc.want)
			}
		})
	}
}

// ----- iouRect -----

func TestIoURect_Table(t *testing.T) {
	tests := []struct {
		name string
		a, b rect
		want float32
	}{
		{"identical rectangles", rect{0, 0, 10, 10}, rect{0, 0, 10, 10}, 1.0},
		{"fully disjoint", rect{0, 0, 10, 10}, rect{20, 20, 30, 30}, 0},
		{"same row no overlap", rect{0, 0, 10, 10}, rect{11, 0, 21, 10}, 0},
		{"edge touch is no overlap", rect{0, 0, 10, 10}, rect{10, 0, 20, 10}, 0},
		{"contained rect", rect{0, 0, 20, 20}, rect{5, 5, 15, 15}, 0.25},
		{"half overlap exact", rect{0, 0, 3, 3}, rect{0, 1, 3, 4}, 0.5},
		{"partial overlap 50/150", rect{0, 0, 10, 10}, rect{5, 0, 15, 10}, float32(50.0 / 150.0)},
		{"zero area rect", rect{5, 5, 5, 5}, rect{0, 0, 10, 10}, 0},
		{"zero width rect", rect{0, 0, 0, 10}, rect{0, 0, 10, 10}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := iouRect(tc.a, tc.b)
			if math.Abs(float64(got-tc.want)) > 0.001 {
				t.Errorf("iouRect(%+v, %+v) = %f, want %f", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// ----- boxToRect -----

func TestBoxToRect_Table(t *testing.T) {
	tests := []struct {
		name string
		box  [4][2]int
		want rect
	}{
		{"axis-aligned", [4][2]int{{10, 20}, {30, 20}, {30, 50}, {10, 50}}, rect{10, 20, 30, 50}},
		{"shuffled corners", [4][2]int{{30, 50}, {10, 20}, {10, 50}, {30, 20}}, rect{10, 20, 30, 50}},
		{"negative coordinates", [4][2]int{{-5, -8}, {3, -8}, {3, 4}, {-5, 4}}, rect{-5, -8, 3, 4}},
		{"degenerate single point", [4][2]int{{7, 7}, {7, 7}, {7, 7}, {7, 7}}, rect{7, 7, 7, 7}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := boxToRect(tc.box); got != tc.want {
				t.Errorf("boxToRect(%v) = %+v, want %+v", tc.box, got, tc.want)
			}
		})
	}
}

// ----- avgBoxScore -----

func TestAvgBoxScore_Table(t *testing.T) {
	tests := []struct {
		name string
		prob []float32
		box  [4][2]int
		mapW int
		mapH int
		want float32
	}{
		{
			name: "uniform map returns constant",
			prob: uniformMap(100, 0.5),
			box:  [4][2]int{{2, 2}, {5, 2}, {5, 5}, {2, 5}},
			mapW: 10, mapH: 10,
			want: 0.5,
		},
		{
			name: "ramp map averages inclusive interior",
			prob: rampMap(4, 4),
			box:  [4][2]int{{1, 1}, {2, 1}, {2, 2}, {1, 2}},
			mapW: 4, mapH: 4,
			// values 5,6,9,10 → 30/4
			want: 7.5,
		},
		{
			name: "box covering whole map",
			prob: rampMap(4, 4),
			box:  [4][2]int{{0, 0}, {3, 0}, {3, 3}, {0, 3}},
			mapW: 4, mapH: 4,
			// (0+...+15)/16
			want: 7.5,
		},
		{
			name: "out-of-bounds box clamped to map",
			prob: rampMap(4, 4),
			box:  [4][2]int{{0, 0}, {100, 0}, {100, 100}, {0, 100}},
			mapW: 4, mapH: 4,
			want: 7.5,
		},
		{
			name: "partially out-of-bounds right/bottom clamped",
			prob: rampMap(5, 5),
			box:  [4][2]int{{2, 2}, {5, 2}, {5, 5}, {2, 5}},
			mapW: 5, mapH: 5,
			// clamped to x,y ∈ 2..4 → (12+13+14 + 17+18+19 + 22+23+24)/9 = 162/9
			want: 18,
		},
		{
			name: "negative min clamped to zero",
			prob: rampMap(4, 4),
			box:  [4][2]int{{-5, -5}, {1, -5}, {1, 1}, {-5, 1}},
			mapW: 4, mapH: 4,
			// x,y ∈ 0..1 → (0+1+4+5)/4
			want: 2.5,
		},
		{
			name: "zero width box returns zero",
			prob: rampMap(4, 4),
			box:  [4][2]int{{3, 3}, {3, 3}, {3, 3}, {3, 3}},
			mapW: 4, mapH: 4,
			want: 0,
		},
		{
			name: "flat line box returns zero",
			prob: rampMap(4, 4),
			box:  [4][2]int{{0, 2}, {3, 2}, {3, 2}, {0, 2}},
			mapW: 4, mapH: 4,
			want: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := avgBoxScore(tc.prob, tc.box, tc.mapW, tc.mapH)
			if math.Abs(float64(got-tc.want)) > 0.001 {
				t.Errorf("avgBoxScore(%v) = %f, want %f", tc.box, got, tc.want)
			}
		})
	}
}

// ----- maxInt / minInt -----

func TestMaxInt_Table(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"first larger", 5, 3, 5},
		{"second larger", 3, 5, 5},
		{"equal", 4, 4, 4},
		{"zero and positive", 0, 1, 1},
		{"negative and positive", -7, 2, 2},
		{"both negative", -9, -2, -2},
		{"int extremes", math.MinInt, math.MaxInt, math.MaxInt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := maxInt(tc.a, tc.b); got != tc.want {
				t.Errorf("maxInt(%d,%d) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestMinInt_Table(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"first smaller", 3, 5, 3},
		{"second smaller", 5, 3, 3},
		{"equal", 4, 4, 4},
		{"zero and positive", 0, 1, 0},
		{"negative and positive", -7, 2, -7},
		{"both negative", -9, -2, -9},
		{"int extremes", math.MinInt, math.MaxInt, math.MinInt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := minInt(tc.a, tc.b); got != tc.want {
				t.Errorf("minInt(%d,%d) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// ----- test helpers -----

func ocrPointsEqual(a, b []ocrPoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalIntSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// rampMap builds a w*h probability map where value at (x,y) = y*w + x.
func rampMap(w, h int) []float32 {
	m := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m[y*w+x] = float32(y*w + x)
		}
	}
	return m
}

func uniformMap(n int, v float32) []float32 {
	m := make([]float32, n)
	for i := range m {
		m[i] = v
	}
	return m
}
