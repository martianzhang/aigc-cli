package background

import "testing"

func TestFeatherAlpha_noop(t *testing.T) {
	alpha := []uint8{0, 255, 128, 64}
	result := featherAlpha(alpha, 2, 2, 0)
	if len(result) != 4 {
		t.Errorf("featherAlpha(radius=0): got %d elements, want 4", len(result))
	}
}

func TestFeatherAlpha_blur(t *testing.T) {
	w, h := 4, 4
	alpha := make([]uint8, w*h)
	alpha[1*4+1] = 255 // single white pixel

	result := featherAlpha(alpha, w, h, 1)

	// Box blur radius=1 averages over 3x3=9 pixels. Center pixel should be ~28 (255/9)
	if result[1*4+1] < 20 || result[1*4+1] > 36 {
		t.Errorf("center pixel: got %d, want ~28", result[1*4+1])
	}
	// Adjacent pixels should have some alpha from the blur
	adjacentCount := 0
	for _, idx := range []int{0*4 + 1, 1*4 + 0, 1*4 + 2, 2*4 + 1} {
		if result[idx] > 0 {
			adjacentCount++
		}
	}
	if adjacentCount == 0 {
		t.Error("blur did not spread to adjacent pixels")
	}
}
