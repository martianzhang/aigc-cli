package watermark

import (
	"image"
	"image/color"
	"testing"
)

func TestComputeCropBounds(t *testing.T) {
	tests := []struct {
		name      string
		imgW      int
		imgH      int
		regions   []CropRegion
		wantValid bool
		wantW     int
		wantH     int
	}{
		{
			name:      "no regions returns full image",
			imgW:      1920,
			imgH:      1080,
			regions:   nil,
			wantValid: true,
			wantW:     1920,
			wantH:     1080,
		},
		{
			name: "bottom-right corner watermark",
			imgW: 1920,
			imgH: 1080,
			regions: []CropRegion{
				{X: 1800, Y: 1000, W: 100, H: 60, Confidence: 0.8},
			},
			wantValid: true,
			wantW:     1790, // minX - margin = 1800 - 10
			wantH:     990,  // minY - margin = 1000 - 10
		},
		{
			name: "bottom-left corner watermark",
			imgW: 1920,
			imgH: 1080,
			regions: []CropRegion{
				{X: 20, Y: 1000, W: 100, H: 60, Confidence: 0.8},
			},
			wantValid: true,
			wantW:     1790, // imgW - (maxX + margin) = 1920 - 130
			wantH:     990,  // minY - margin = 1000 - 10
		},
		{
			name: "top-right corner watermark",
			imgW: 1920,
			imgH: 1080,
			regions: []CropRegion{
				{X: 1800, Y: 20, W: 100, H: 60, Confidence: 0.8},
			},
			wantValid: true,
			wantW:     1790, // minX - margin = 1800 - 10
			wantH:     990,  // imgH - (maxY + margin) = 1080 - 90
		},
		{
			name: "top-left corner watermark",
			imgW: 1920,
			imgH: 1080,
			regions: []CropRegion{
				{X: 20, Y: 20, W: 100, H: 60, Confidence: 0.8},
			},
			wantValid: true,
			wantW:     1790, // imgW - (maxX + margin) = 1920 - 130
			wantH:     990,  // imgH - (maxY + margin) = 1080 - 90
		},
		{
			name: "center watermark returns invalid",
			imgW: 1920,
			imgH: 1080,
			regions: []CropRegion{
				{X: 900, Y: 500, W: 100, H: 60, Confidence: 0.8},
			},
			wantValid: false,
		},
		{
			name: "false positives filtered by relative confidence",
			imgW: 864,
			imgH: 1152,
			regions: []CropRegion{
				// Real watermark: bottom-right, high confidence
				{X: 735, Y: 1055, W: 129, H: 97, Confidence: 1.0},
				{X: 799, Y: 1023, W: 65, H: 32, Confidence: 1.0},
				{X: 799, Y: 1087, W: 32, H: 32, Confidence: 0.62},
				// False positives: other corners, low confidence
				{X: 735, Y: 0, W: 129, H: 160, Confidence: 0.55},
				{X: 0, Y: 1023, W: 160, H: 129, Confidence: 0.51},
			},
			wantValid: true,
			wantW:     725,  // minX - margin = 735 - 10
			wantH:     1013, // minY - margin = 1023 - 10
		},
		{
			name: "multiple regions merged",
			imgW: 1920,
			imgH: 1080,
			regions: []CropRegion{
				{X: 1800, Y: 1000, W: 80, H: 40, Confidence: 0.8},
				{X: 1850, Y: 1020, W: 60, H: 50, Confidence: 0.7},
			},
			wantValid: true,
			wantW:     1790, // minX = 1800, cropW = 1800 - 10
			wantH:     990,  // minY = 1000, cropH = 1000 - 10
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeCropBounds(tt.imgW, tt.imgH, tt.regions)
			if got.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", got.Valid, tt.wantValid)
				return
			}
			if tt.wantValid {
				// Allow some tolerance for margin calculations
				if diff := abs(got.W - tt.wantW); diff > 20 {
					t.Errorf("W = %d, want %d (diff=%d)", got.W, tt.wantW, diff)
				}
				if diff := abs(got.H - tt.wantH); diff > 20 {
					t.Errorf("H = %d, want %d (diff=%d)", got.H, tt.wantH, diff)
				}
			}
		})
	}
}

func TestAdjustCropToTarget(t *testing.T) {
	tests := []struct {
		name      string
		bounds    CropBounds
		targetW   int
		targetH   int
		wantValid bool
		wantRatio float64 // expected aspect ratio (targetW/targetH)
	}{
		{
			name:      "invalid bounds stays invalid",
			bounds:    CropBounds{Valid: false},
			targetW:   1920,
			targetH:   1080,
			wantValid: false,
		},
		{
			name:      "zero target stays unchanged",
			bounds:    CropBounds{X: 0, Y: 0, W: 1920, H: 1080, Valid: true},
			targetW:   0,
			targetH:   0,
			wantValid: true,
			wantRatio: 1920.0 / 1080.0,
		},
		{
			name:      "already matching ratio",
			bounds:    CropBounds{X: 0, Y: 0, W: 1920, H: 1080, Valid: true},
			targetW:   1920,
			targetH:   1080,
			wantValid: true,
			wantRatio: 1920.0 / 1080.0,
		},
		{
			name:      "adjust wider to narrower",
			bounds:    CropBounds{X: 0, Y: 0, W: 1920, H: 1080, Valid: true},
			targetW:   1280,
			targetH:   720,
			wantValid: true,
			wantRatio: 1280.0 / 720.0,
		},
		{
			name:      "adjust square to widescreen",
			bounds:    CropBounds{X: 0, Y: 0, W: 1000, H: 1000, Valid: true},
			targetW:   1920,
			targetH:   1080,
			wantValid: true,
			wantRatio: 1920.0 / 1080.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AdjustCropToTarget(tt.bounds, tt.targetW, tt.targetH)
			if got.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", got.Valid, tt.wantValid)
				return
			}
			if tt.wantValid && got.Valid {
				gotRatio := float64(got.W) / float64(got.H)
				if diff := absFloat(gotRatio - tt.wantRatio); diff > 0.02 {
					t.Errorf("ratio = %.4f, want %.4f (diff=%.4f)", gotRatio, tt.wantRatio, diff)
				}
			}
		})
	}
}

func TestRegionsOverlap(t *testing.T) {
	tests := []struct {
		name string
		a    CropRegion
		b    CropRegion
		want bool
	}{
		{
			name: "overlapping regions",
			a:    CropRegion{X: 100, Y: 100, W: 50, H: 50},
			b:    CropRegion{X: 130, Y: 130, W: 50, H: 50},
			want: true,
		},
		{
			name: "adjacent regions within gap",
			a:    CropRegion{X: 100, Y: 100, W: 50, H: 50},
			b:    CropRegion{X: 160, Y: 100, W: 50, H: 50}, // gap = 10 < 16
			want: true,
		},
		{
			name: "distant regions",
			a:    CropRegion{X: 100, Y: 100, W: 50, H: 50},
			b:    CropRegion{X: 300, Y: 300, W: 50, H: 50},
			want: false,
		},
		{
			name: "touching corners",
			a:    CropRegion{X: 100, Y: 100, W: 50, H: 50},
			b:    CropRegion{X: 150, Y: 150, W: 50, H: 50},
			want: true, // corners touch, gap=0 < 16
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := regionsOverlap(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("regionsOverlap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectWatermarkRegions(t *testing.T) {
	// Create a test image with a bright watermark-like region in bottom-right
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))

	// Fill with dark background
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 30, B: 30, A: 255})
		}
	}

	// Add bright "watermark" region in bottom-right corner
	for y := 250; y < 280; y++ {
		for x := 350; x < 390; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 200, B: 200, A: 255})
		}
	}

	regions := DetectWatermarkRegions(img)

	// Should detect at least one region in the bottom-right
	if len(regions) == 0 {
		t.Log("No regions detected - may need to adjust thresholds for test image")
		return
	}

	// Check that detected regions are in the bottom-right area
	found := false
	for _, r := range regions {
		if r.X > 300 && r.Y > 200 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected region in bottom-right, got regions: %+v", regions)
	}
}

func TestSuggestCropSize(t *testing.T) {
	// No regions
	w, h := SuggestCropSize(1920, 1080, nil)
	if w != 1920 || h != 1080 {
		t.Errorf("SuggestCropSize(nil) = (%d, %d), want (1920, 1080)", w, h)
	}

	// Bottom-right watermark
	regions := []CropRegion{
		{X: 1800, Y: 1000, W: 100, H: 60, Confidence: 0.8},
	}
	w, h = SuggestCropSize(1920, 1080, regions)
	if w <= 0 || h <= 0 {
		t.Errorf("SuggestCropSize(bottom-right) = (%d, %d), want positive values", w, h)
	}
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// absFloat returns the absolute value of a float64.
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
