package detect

import (
	"image"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/forensic"
	"github.com/martianzhang/aigc-cli/internal/watermark"
)

func TestParseLLMScore(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{"slash separate token", "Score: 85 /100", 0.85},
		{"percent", "85%", 0.85},
		{"full percent", "100%", 1.0},
		{"bare number is not a score", "42", 0.5},
		{"out of range ignored", "150%", 0.5},
		{"ai keyword", "This is artificial", 0.8},
		{"synthetic deepfake", "clearly AI-generated synthetic deepfake", 1.0},
		{"human keywords", "human-made authentic realistic", 0.0},
		{"no signal", "", 0.5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseLLMScore(tc.in); got != tc.want {
				t.Errorf("ParseLLMScore(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildDetails(t *testing.T) {
	t.Run("empty signals", func(t *testing.T) {
		if got := BuildDetails(&forensic.Result{}); got != "" {
			t.Errorf("BuildDetails(empty) = %q, want %q", got, "")
		}
	})

	t.Run("signals joined with percent", func(t *testing.T) {
		r := &forensic.Result{Signals: []forensic.SignalResult{
			{Name: "A", Score: 0.5},
			{Name: "B", Score: 1.0},
		}}
		want := "A=50%; B=100%"
		if got := BuildDetails(r); got != want {
			t.Errorf("BuildDetails = %q, want %q", got, want)
		}
	})
}

func TestParseWatermarkBox(t *testing.T) {
	t.Run("width height bottom-right", func(t *testing.T) {
		x, y, w, h, ok := ParseWatermarkBox("200,60", 1000, 800)
		if !ok || x != 790 || y != 730 || w != 200 || h != 60 {
			t.Fatalf("got (%d,%d,%d,%d,%v), want (790,730,200,60,true)", x, y, w, h, ok)
		}
	})

	t.Run("explicit box", func(t *testing.T) {
		x, y, w, h, ok := ParseWatermarkBox("100,200,300,50", 1000, 800)
		if !ok || x != 100 || y != 200 || w != 300 || h != 50 {
			t.Fatalf("got (%d,%d,%d,%d,%v), want (100,200,300,50,true)", x, y, w, h, ok)
		}
	})

	t.Run("negative offsets clamp and shrink", func(t *testing.T) {
		x, y, w, h, ok := ParseWatermarkBox("-100,-50,300,50", 1000, 800)
		if !ok || x != 900 || y != 750 || w != 100 || h != 50 {
			t.Fatalf("got (%d,%d,%d,%d,%v), want (900,750,100,50,true)", x, y, w, h, ok)
		}
	})

	for _, bad := range []string{"0,60", "abc,60", "200", "1,2,3", "a,b,c,d", "10,20,0,50"} {
		t.Run("invalid "+bad, func(t *testing.T) {
			if _, _, _, _, ok := ParseWatermarkBox(bad, 1000, 800); ok {
				t.Fatalf("ParseWatermarkBox(%q) expected ok=false", bad)
			}
		})
	}
}

func TestParseCropTarget(t *testing.T) {
	t.Run("percentage", func(t *testing.T) {
		w, h, ratio, err := ParseCropTarget("90%")
		if err != nil || w != 0 || h != 0 || ratio != 0.9 {
			t.Fatalf("got (%d,%d,%v,%v), want (0,0,0.9,nil)", w, h, ratio, err)
		}
	})

	t.Run("dimensions", func(t *testing.T) {
		w, h, ratio, err := ParseCropTarget("1920x1080")
		if err != nil || w != 1920 || h != 1080 || ratio != 0 {
			t.Fatalf("got (%d,%d,%v,%v), want (1920,1080,0,nil)", w, h, ratio, err)
		}
	})

	t.Run("uppercase and spaces", func(t *testing.T) {
		w, h, ratio, err := ParseCropTarget(" 800 X 600 ")
		if err != nil || w != 800 || h != 600 || ratio != 0 {
			t.Fatalf("got (%d,%d,%v,%v), want (800,600,0,nil)", w, h, ratio, err)
		}
	})

	for _, bad := range []string{"0%", "101%", "abc%", "0x100", "1920x", "90", "abc"} {
		t.Run("invalid "+bad, func(t *testing.T) {
			if _, _, _, err := ParseCropTarget(bad); err == nil {
				t.Fatalf("ParseCropTarget(%q) expected error", bad)
			}
		})
	}
}

func TestResolveWMBox(t *testing.T) {
	t.Run("manual flag override", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 1000, 800))
		x, y, w, h, ok := ResolveWMBox("200,60", "", img, nil)
		if !ok || x != 790 || y != 730 || w != 200 || h != 60 {
			t.Fatalf("got (%d,%d,%d,%d,%v), want (790,730,200,60,true)", x, y, w, h, ok)
		}
	})

	t.Run("detection box", func(t *testing.T) {
		dets := []watermark.Detection{{X: 10, Y: 20, W: 100, H: 50}}
		x, y, w, h, ok := ResolveWMBox("", "", nil, dets)
		if !ok || x != 10 || y != 20 || w != 100 || h != 50 {
			t.Fatalf("got (%d,%d,%d,%d,%v), want (10,20,100,50,true)", x, y, w, h, ok)
		}
	})

	t.Run("detection size fallback", func(t *testing.T) {
		dets := []watermark.Detection{{X: 5, Y: 6, Size: 30}}
		x, y, w, h, ok := ResolveWMBox("", "", nil, dets)
		if !ok || x != 5 || y != 6 || w != 30 || h != 30 {
			t.Fatalf("got (%d,%d,%d,%d,%v), want (5,6,30,30,true)", x, y, w, h, ok)
		}
	})

	t.Run("no source", func(t *testing.T) {
		if _, _, _, _, ok := ResolveWMBox("", "", nil, nil); ok {
			t.Fatal("want ok=false when no box source is available")
		}
	})
}
