package docredact

import (
	"image"
	"image/color"
	"testing"
)

func TestGlyphSegmentsSnapToInkGaps(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.White)
		}
	}
	want := []image.Rectangle{
		image.Rect(10, 5, 20, 35),
		image.Rect(30, 5, 40, 35),
		image.Rect(50, 5, 60, 35),
		image.Rect(70, 5, 80, 35),
	}
	for _, r := range want {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.Set(x, y, color.Black)
			}
		}
	}
	segs, precise := glyphSegments(img, "张三李四", image.Rect(5, 0, 85, 40), 0)
	if !precise {
		t.Fatal("expected precise segmentation")
	}
	if len(segs) != len(want) {
		t.Fatalf("segments = %d, want %d: %+v", len(segs), len(want), segs)
	}
	for i, seg := range segs {
		if absInt(seg.rect.Min.X-want[i].Min.X) > 2 || absInt(seg.rect.Max.X-want[i].Max.X) > 2 {
			t.Errorf("segment %d = %v, want near %v", i, seg.rect, want[i])
		}
	}
}

func TestTokenBoxesSkipLabels(t *testing.T) {
	text := "电话13812345678"
	segs := runeSegments(0, text, image.Rect(0, 0, 200, 40))
	tokens := tokenBoxes(text, segs)
	if len(tokens) != 1 {
		t.Fatalf("tokens = %+v", tokens)
	}
	if tokens[0].start != len("电话") || tokens[0].end != len(text) {
		t.Fatalf("token = %+v, want only the number", tokens[0])
	}
}

func TestGlyphSegmentsSplitTouchingGlyphs(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.White)
		}
	}
	// The middle two glyphs touch, so OCR pixels yield three components for
	// four characters. The splitter must recover the missing boundary.
	for _, r := range []image.Rectangle{
		image.Rect(10, 5, 20, 35),
		image.Rect(30, 5, 50, 35),
		image.Rect(60, 5, 70, 35),
	} {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.Set(x, y, color.Black)
			}
		}
	}
	segs, precise := glyphSegments(img, "ABCD", image.Rect(5, 0, 75, 40), 0)
	if !precise {
		t.Fatal("expected precise segmentation")
	}
	if len(segs) != 4 {
		t.Fatalf("segments = %d, want 4: %+v", len(segs), segs)
	}
	if segs[1].rect.Max.X != segs[2].rect.Min.X {
		t.Fatalf("middle split is not contiguous: %v %v", segs[1].rect, segs[2].rect)
	}
	if absInt(segs[1].rect.Max.X-40) > 2 {
		t.Fatalf("middle boundary = %d, want near 40", segs[1].rect.Max.X)
	}
}

func TestMergeTouchingUnits(t *testing.T) {
	got := mergeTouchingUnits([]inkBox{{10, 20}, {30, 35}, {35, 45}, {50, 60}})
	want := []inkBox{{10, 20}, {30, 45}, {50, 60}}
	if len(got) != len(want) {
		t.Fatalf("units = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unit %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestRectForRange(t *testing.T) {
	segs := []ocrSegment{
		{start: 0, end: 3, rect: image.Rect(0, 0, 10, 10)},
		{start: 3, end: 6, rect: image.Rect(10, 0, 20, 10)},
		{start: 6, end: 9, rect: image.Rect(20, 0, 30, 10)},
	}
	cases := []struct {
		start, end int
		want       image.Rectangle
	}{
		{0, 9, image.Rect(0, 0, 30, 10)},
		{3, 9, image.Rect(10, 0, 30, 10)},
		{4, 5, image.Rect(10, 0, 20, 10)},
	}
	for _, c := range cases {
		got, ok := rectForRange(segs, c.start, c.end)
		if !ok || got != c.want {
			t.Errorf("rectForRange(%d,%d) = %v,%v want %v", c.start, c.end, got, ok, c.want)
		}
	}
	if _, ok := rectForRange(segs, 9, 12); ok {
		t.Fatal("expected no range outside segments")
	}
}

func TestExpandRectAdaptsToTextSize(t *testing.T) {
	small := expandRect(image.Rect(10, 10, 50, 24))
	if small != image.Rect(8, 8, 52, 26) {
		t.Fatalf("small padding = %v", small)
	}
	large := expandRect(image.Rect(10, 10, 200, 50))
	if large != image.Rect(6, 6, 204, 54) {
		t.Fatalf("large padding = %v", large)
	}
}
