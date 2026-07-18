package background

import (
	"testing"
)

func TestFindBounds(t *testing.T) {
	w, h := 10, 10
	alpha := make([]uint8, w*h)

	// All transparent
	_, _, _, _, ok := findBounds(alpha, w, h)
	if ok {
		t.Error("findBounds should return false for all-transparent mask")
	}

	// One opaque pixel
	alpha[3*10+4] = 255
	x0, y0, x1, y1, ok := findBounds(alpha, w, h)
	if !ok {
		t.Fatal("findBounds should return true for non-empty mask")
	}
	if x0 != 4 || y0 != 3 || x1 != 4 || y1 != 3 {
		t.Errorf("findBounds got (%d,%d)-(%d,%d), want (4,3)-(4,3)", x0, y0, x1, y1)
	}

	// Multiple opaque pixels
	alpha[1*10+2] = 100
	x0, y0, x1, y1, ok = findBounds(alpha, w, h)
	if !ok {
		t.Fatal("findBounds should return true for non-empty mask")
	}
	if x0 != 2 || y0 != 1 || x1 != 4 || y1 != 3 {
		t.Errorf("findBounds multiple got (%d,%d)-(%d,%d)", x0, y0, x1, y1)
	}
}

func TestApplyPadding(t *testing.T) {
	imgW, imgH := 100, 100
	x0, y0, x1, y1 := 20, 20, 80, 80

	// Uniform padding
	nx0, ny0, nx1, ny1 := applyPadding(x0, y0, x1, y1, imgW, imgH, [4]int{10, 10, 10, 10})
	if nx0 != 10 || ny0 != 10 || nx1 != 90 || ny1 != 90 {
		t.Errorf("uniform padding got (%d,%d)-(%d,%d), want (10,10)-(90,90)", nx0, ny0, nx1, ny1)
	}

	// Clamp to image bounds
	nx0, ny0, nx1, ny1 = applyPadding(0, 0, 95, 95, 100, 100, [4]int{10, 10, 10, 10})
	if nx0 != 0 || ny0 != 0 || nx1 != 99 || ny1 != 99 {
		t.Errorf("clamp padding got (%d,%d)-(%d,%d)", nx0, ny0, nx1, ny1)
	}
}

func TestApplyAspectRatio(t *testing.T) {
	imgW, imgH := 100, 100

	// Empty ratio → unchanged
	x0, y0, x1, y1 := applyAspectRatio(10, 10, 30, 30, imgW, imgH, "")
	if x0 != 10 || y0 != 10 || x1 != 30 || y1 != 30 {
		t.Errorf("empty ratio modified bounds: (%d,%d)-(%d,%d)", x0, y0, x1, y1)
	}

	// 1:1 ratio on 20x10 rect → expand height
	x0, y0, x1, y1 = applyAspectRatio(10, 15, 30, 25, imgW, imgH, "1:1")
	// 20x10 → need width==height, expand height to 20, centered
	if x1-x0+1 != 21 || y1-y0+1 != 21 {
		t.Errorf("1:1 ratio got (%d,%d)-(%d,%d), area %dx%d", x0, y0, x1, y1, x1-x0+1, y1-y0+1)
	}
}

func TestCropImage(t *testing.T) {
	w, h := 10, 10
	pixels := make([]uint8, w*h*4)
	// Fill with red
	for i := 0; i < len(pixels); i += 4 {
		pixels[i] = 255
		pixels[i+1] = 0
		pixels[i+2] = 0
		pixels[i+3] = 255
	}

	cropped, cw, ch := cropImage(pixels, w, h, 2, 2, 7, 7)
	if cw != 6 || ch != 6 {
		t.Errorf("cropImage: got %dx%d, want 6x6", cw, ch)
	}
	if len(cropped) != 6*6*4 {
		t.Errorf("cropImage: output pixels length %d, want %d", len(cropped), 6*6*4)
	}
	// Check first pixel
	if cropped[0] != 255 || cropped[1] != 0 || cropped[2] != 0 {
		t.Errorf("cropImage: first pixel changed")
	}
}
