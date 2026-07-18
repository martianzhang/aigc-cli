package background

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	opts := Defaults()
	if opts.Autocrop {
		t.Error("Autocrop should default to false")
	}
	if opts.AspectRatio != "" {
		t.Errorf("AspectRatio should default to empty, got %q", opts.AspectRatio)
	}
	if opts.Shadow {
		t.Error("Shadow should default to false")
	}
	if opts.ShadowOffset != [2]int{4, 4} {
		t.Errorf("ShadowOffset should default to {4,4}, got %v", opts.ShadowOffset)
	}
	if opts.ShadowBlur != 6 {
		t.Errorf("ShadowBlur should default to 6, got %d", opts.ShadowBlur)
	}
	if opts.ShadowOpacity != 40 {
		t.Errorf("ShadowOpacity should default to 40, got %f", opts.ShadowOpacity)
	}
}

func TestParsePadding(t *testing.T) {
	tests := []struct {
		input string
		want  [4]int
	}{
		{"20", [4]int{20, 20, 20, 20}},
		{"10,20,30,40", [4]int{10, 20, 30, 40}},
		{"", [4]int{0, 0, 0, 0}},
	}
	for _, tt := range tests {
		got, err := ParsePadding(tt.input)
		if err != nil {
			t.Errorf("ParsePadding(%q): unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParsePadding(%q): got %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParsePadding_invalid(t *testing.T) {
	_, err := ParsePadding("a,b,c")
	if err == nil {
		t.Error("ParsePadding should return error for invalid input")
	}
}

func TestClampByte(t *testing.T) {
	tests := []struct {
		input float64
		want  uint8
	}{
		{-1, 0},
		{0, 0},
		{128, 128},
		{255, 255},
		{300, 255},
	}
	for _, tt := range tests {
		got := clampByte(tt.input)
		if got != tt.want {
			t.Errorf("clampByte(%f): got %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestSavePNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))

	err := SavePNG(path, img)
	if err != nil {
		t.Fatalf("SavePNG: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("SavePNG: file not created: %v", err)
	}
}

func TestCompositeOnColor(t *testing.T) {
	fg := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	// Make fg fully transparent
	for i := range fg.Pix {
		fg.Pix[i] = 0
	}
	// Set one pixel opaque red
	fg.Pix[0] = 255 // R
	fg.Pix[1] = 0   // G
	fg.Pix[2] = 0   // B
	fg.Pix[3] = 255 // A

	bg := color.NRGBA{R: 0, G: 255, B: 0, A: 255} // green
	result := compositeOnColor(fg, bg)

	if result.Bounds().Dx() != 10 || result.Bounds().Dy() != 10 {
		t.Errorf("compositeOnColor: output size got %dx%d, want 10x10", result.Bounds().Dx(), result.Bounds().Dy())
	}

	// Red pixel should stay red (fg over bg)
	if result.Pix[0] != 255 || result.Pix[1] != 0 || result.Pix[2] != 0 {
		t.Errorf("compositeOnColor: opaque FG pixel changed: got [%d,%d,%d]", result.Pix[0], result.Pix[1], result.Pix[2])
	}
}

func TestCompositeOnImage(t *testing.T) {
	fg := image.NewNRGBA(image.Rect(0, 0, 5, 5))
	bg := image.NewRGBA(image.Rect(0, 0, 5, 5))

	result := compositeOnImage(fg, bg)
	if result.Bounds().Dx() != 5 || result.Bounds().Dy() != 5 {
		t.Errorf("compositeOnImage: output size got %dx%d, want 5x5", result.Bounds().Dx(), result.Bounds().Dy())
	}
}
