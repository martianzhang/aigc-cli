package upscale

import (
	"image"
	"image/color"
	"math"
	"path/filepath"
	"testing"
)

func solidNRGBA(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestResolveModel(t *testing.T) {
	if m, ok := ResolveModel(""); !ok || m.ID != DefaultModelID {
		t.Fatalf("empty id should resolve to default, got %q ok=%v", m.ID, ok)
	}
	if m, ok := ResolveModel("real-cugan-2x"); !ok || m.Scale != 2 {
		t.Fatalf("real-cugan-2x should resolve scale 2, got %+v ok=%v", m, ok)
	}
	if _, ok := ResolveModel("does-not-exist"); ok {
		t.Fatal("unknown id must not resolve")
	}
}

func TestLookupAndURL(t *testing.T) {
	if _, err := Lookup("nope"); err == nil {
		t.Fatal("Lookup of unknown id should error")
	}
	m, err := Lookup("realesr-general-x4v3")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := URL(m), modelsBaseURL+"/upscale-"+m.File; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
	if filepath.Base(ModelPath("/root", "realesr-general-x4v3")) != m.File {
		t.Fatalf("ModelPath basename = %q, want %q", filepath.Base(ModelPath("/root", m.ID)), m.File)
	}
}

func TestExtractTileNormalizesAndClamps(t *testing.T) {
	img := solidNRGBA(4, 4, color.NRGBA{R: 255, G: 128, B: 0, A: 255})
	buf := make([]float32, 3*2*2)

	ExtractTile(img, 0, 0, 2, buf)
	if math.Abs(float64(buf[0]-1.0)) > 1e-3 {
		t.Fatalf("R = %v, want ~1.0", buf[0])
	}
	if math.Abs(float64(buf[4]-128.0/255.0)) > 1e-3 {
		t.Fatalf("G = %v, want ~%v", buf[4], 128.0/255.0)
	}
	if buf[8] != 0 {
		t.Fatalf("B = %v, want 0", buf[8])
	}

	edge := make([]float32, 3*2*2)
	ExtractTile(img, 5, 5, 2, edge)
	for i := range buf {
		if buf[i] != edge[i] {
			t.Fatalf("edge tile should clamp to same color: buf[%d]=%v edge=%v", i, buf[i], edge[i])
		}
	}
}

func TestBlendWeights(t *testing.T) {
	full, ramp := BlendWeights(4, 1, 2)
	if len(full) != 4 || len(ramp) != 4 {
		t.Fatalf("len full=%d ramp=%d, want 4", len(full), len(ramp))
	}
	for i, v := range full {
		if v != 1 {
			t.Fatalf("full[%d]=%v, want 1", i, v)
		}
	}
	want := []float32{0, 0.5, 1, 1}
	for i, w := range want {
		if math.Abs(float64(ramp[i]-w)) > 1e-6 {
			t.Fatalf("ramp[%d]=%v, want %v", i, ramp[i], w)
		}
	}
}

func TestBlendTileWritesOpaquePixels(t *testing.T) {
	dst := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	tile := []float32{0.5, 0.5, 0.5, 0.5, 0, 0, 0, 0, 1, 1, 1, 1}
	ax := []float32{1, 1}
	BlendTile(dst, tile, 0, 0, 2, 2, 1, ax, ax)

	got := dst.NRGBAAt(0, 0)
	if got.R != 127 || got.G != 0 || got.B != 255 || got.A != 255 {
		t.Fatalf("pixel = %+v, want R127 G0 B255 A255", got)
	}
}

func TestTileCount(t *testing.T) {
	cases := []struct{ n, step, want int }{
		{256, 240, 1},
		{257, 240, 2},
		{500, 240, 3},
		{1000, 240, 5},
		{100, 240, 1},
	}
	for _, c := range cases {
		if got := tileCount(c.n, c.step); got != c.want {
			t.Fatalf("tileCount(%d,%d)=%d, want %d", c.n, c.step, got, c.want)
		}
	}
}
