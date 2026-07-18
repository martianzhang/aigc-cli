package background

import (
	"image/color"
	"testing"
)

func TestParseShadowConfig_disabled(t *testing.T) {
	opts := Defaults()
	opts.Shadow = false
	cfg := parseShadowConfig(&opts)
	if cfg.enabled {
		t.Error("shadow should be disabled")
	}
}

func TestParseShadowConfig_defaults(t *testing.T) {
	opts := Defaults()
	opts.Shadow = true
	cfg := parseShadowConfig(&opts)
	if !cfg.enabled {
		t.Fatal("shadow should be enabled")
	}
	if cfg.offsetX != 4 || cfg.offsetY != 4 {
		t.Errorf("offset got (%d,%d), want (4,4)", cfg.offsetX, cfg.offsetY)
	}
	if cfg.blur != 6 {
		t.Errorf("blur got %d, want 6", cfg.blur)
	}
	if cfg.col.R != 0 || cfg.col.G != 0 || cfg.col.B != 0 {
		t.Errorf("shadow color should be black, got (%d,%d,%d)", cfg.col.R, cfg.col.G, cfg.col.B)
	}
}

func TestParseShadowConfig_custom(t *testing.T) {
	opts := Defaults()
	opts.Shadow = true
	opts.ShadowOffset = [2]int{8, 12}
	opts.ShadowBlur = 10
	opts.ShadowColor = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
	opts.ShadowOpacity = 75

	cfg := parseShadowConfig(&opts)
	if cfg.offsetX != 8 || cfg.offsetY != 12 {
		t.Errorf("offset got (%d,%d)", cfg.offsetX, cfg.offsetY)
	}
	if cfg.blur != 10 {
		t.Errorf("blur got %d", cfg.blur)
	}
	if cfg.col.R != 255 || cfg.col.G != 0 || cfg.col.B != 0 {
		t.Errorf("color got (%d,%d,%d)", cfg.col.R, cfg.col.G, cfg.col.B)
	}
	// Opacity: 75/100 * 255 = 191
	if cfg.col.A != 191 {
		t.Errorf("opacity got %d, want 191", cfg.col.A)
	}
}

func TestParseShadowConfig_clampOpacity(t *testing.T) {
	opts := Defaults()
	opts.Shadow = true
	opts.ShadowOpacity = 120
	cfg := parseShadowConfig(&opts)
	if cfg.col.A != 255 {
		t.Errorf("clamped opacity got %d, want 255", cfg.col.A)
	}
}

func TestApplyShadow(t *testing.T) {
	w, h := 8, 8
	pixels := make([]uint8, w*h*4)
	// Fully transparent image
	for i := range pixels {
		pixels[i] = 0
	}
	alpha := make([]uint8, w*h)
	// Single opaque pixel in center
	alpha[4*8+4] = 255

	cfg := shadowConfig{
		enabled: true,
		offsetX: 1,
		offsetY: 1,
		blur:    0,
		col:     color.NRGBA{R: 0, G: 0, B: 0, A: 102}, // 40% opacity
	}

	applyShadow(pixels, w, h, alpha, cfg)

	// The shadow should appear at offset (1,1) from the center pixel
	shadowIdx := (5*8 + 5) * 4 // center + offset
	if pixels[shadowIdx+3] == 0 {
		t.Error("shadow pixel should have alpha > 0")
	}
}
