package depth

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// testGradient creates a WxH image that is bright at the bottom (near)
// and dark at the top (far), approximating a simple depth scene.
func testGradient(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		v := uint8(255 - y*255/h)
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

func TestPreprocessShape(t *testing.T) {
	img := testGradient(700, 700)
	pixels := Preprocess(img)
	want := ModelChannels * ModelInputSize * ModelInputSize
	if len(pixels) != want {
		t.Fatalf("Preprocess length = %d, want %d", len(pixels), want)
	}

	// ImageNet normalization sanity: top-left is white (255) in the gradient:
	// (255/255 - 0.485)/0.229 ≈ 2.249.
	idx := 0 // first channel (R), first pixel (top-left, white)
	if math.Abs(float64(pixels[idx])-2.249) > 0.05 {
		t.Errorf("top-left (white) normalized = %v, want ≈ 2.249", pixels[idx])
	}
}

func TestPreprocessToAligned(t *testing.T) {
	img := testGradient(640, 480)
	pixels, crop := PreprocessCrop(img, 518)
	want := 3 * 518 * 518
	if len(pixels) != want {
		t.Fatalf("PreprocessCrop length = %d, want %d", len(pixels), want)
	}
	// 640x480 (4:3): long side 480 → scaled to 518? No — long side 640 scales
	// to 518, short side 480 → 388. So crop is 518 wide, 388 tall, padded
	// vertically. Verify crop region is inside the canvas.
	if crop.Width > 518 || crop.Height > 518 {
		t.Fatalf("crop %dx%d exceeds canvas 518x518", crop.Width, crop.Height)
	}
}

func TestPreprocessAspectPreserving(t *testing.T) {
	// 640x360 (16:9 landscape). Long side 640 scales to canvas 378:
	// resizedW = 378, resizedH = 360*378/640 = 212. Padded vertically.
	img := testGradient(640, 360)
	pixels, crop := PreprocessCrop(img, 378)
	if len(pixels) != 3*378*378 {
		t.Fatalf("length = %d, want %d", len(pixels), 3*378*378)
	}
	if crop.Width != 378 || crop.Height != 212 {
		t.Fatalf("crop = %dx%d, want 378x212", crop.Width, crop.Height)
	}
	// Aspect ratio preserved inside the crop: 378/212 ≈ 1.78 ≈ 16:9.
	if math.Abs(float64(crop.Width)/float64(crop.Height)-640.0/360.0) > 0.01 {
		t.Errorf("crop aspect %d:%d deviates from source %d:%d", crop.Width, crop.Height, 640, 360)
	}

	// A 1:1 input at target 378 stays square with no pad: crop fills canvas.
	sq := testGradient(378, 378)
	pixSq, cropSq := PreprocessCrop(sq, 378)
	if cropSq.Width != 378 || cropSq.Height != 378 {
		t.Fatalf("square crop = %dx%d, want 378x378 (no pad)", cropSq.Width, cropSq.Height)
	}
	// Top-left normalized R for white ≈ (1-0.485)/0.229 ≈ 2.249.
	if math.Abs(float64(pixSq[0])-2.249) > 0.1 {
		t.Errorf("square input top-left = %v, want ≈2.249 (no pad expected)", pixSq[0])
	}
}

func TestDepthToGrayCrop(t *testing.T) {
	// Simulate: canvas 4x4, crop is right half (X=2,Y=0,W=2,H=4).
	// data: 4x4 with all-ones except a bright spot in the crop region.
	data := make([]float32, 16)
	for i := range data {
		data[i] = 1.0
	}
	data[2] = 3.0 // top-right of crop region → brightest
	crop := Crop{X: 2, Y: 0, Width: 2, Height: 4}
	gray := DepthToGrayCrop(data, 4, crop, 2, 4)
	if gray.Bounds().Dx() != 2 || gray.Bounds().Dy() != 4 {
		t.Fatalf("gray size = %v, want 2x4", gray.Bounds())
	}
	// The bright pixel (canvas idx 2) maps to crop-local (0,0) → gray (0,0) = 255.
	if got := gray.GrayAt(0, 0).Y; got != 255 {
		t.Errorf("crop top-left = %d, want 255 (bright spot)", got)
	}
	// Crop-local (1,0) is canvas idx 3 (value 1.0) → darkest (0).
	if got := gray.GrayAt(1, 0).Y; got != 0 {
		t.Errorf("crop neighbor = %d, want 0", got)
	}
}

func TestDepthToGraySameSize(t *testing.T) {
	// 2x2 inverse depth: near (high) at bottom, far (low) at top.
	data := []float32{0.5, 0.5, 2.0, 3.0} // row-major, 2x2
	gray := DepthToGray(data, 2, 2, 2, 2)

	if gray.Bounds().Dx() != 2 || gray.Bounds().Dy() != 2 {
		t.Fatalf("gray size = %v, want 2x2", gray.Bounds())
	}
	// bottom-right (max 3.0) should be brightest (255)
	if got := gray.GrayAt(1, 1).Y; got != 255 {
		t.Errorf("near pixel = %d, want 255", got)
	}
	// top-left (min 0.5) should be darkest (0)
	if got := gray.GrayAt(0, 0).Y; got != 0 {
		t.Errorf("far pixel = %d, want 0", got)
	}
	// near > far throughout
	if gray.GrayAt(0, 1).Y <= gray.GrayAt(1, 0).Y {
		t.Errorf("depth ordering violated: bottom=%d top-right=%d", gray.GrayAt(0, 1).Y, gray.GrayAt(1, 0).Y)
	}
}

func TestDepthToGrayResize(t *testing.T) {
	// 2x2 depth resized up to 4x4 keeps near-bright/far-dark ordering.
	data := []float32{0.5, 0.5, 0.5, 3.0} // bottom-right is near
	gray := DepthToGray(data, 2, 2, 4, 4)

	if gray.Bounds().Dx() != 4 || gray.Bounds().Dy() != 4 {
		t.Fatalf("gray size = %v, want 4x4", gray.Bounds())
	}
	// bottom-right region should be brighter than top-left region.
	bottomRight := gray.GrayAt(3, 3).Y
	topLeft := gray.GrayAt(0, 0).Y
	if bottomRight <= topLeft {
		t.Errorf("resize lost depth ordering: bottomRight=%d topLeft=%d", bottomRight, topLeft)
	}
}

func TestDepthToGrayConstant(t *testing.T) {
	// Constant input → all 0 (or all 255 since lo==hi). Either is fine,
	// but it must not panic or produce NaN.
	data := make([]float32, 4)
	for i := range data {
		data[i] = 1.0
	}
	gray := DepthToGray(data, 2, 2, 2, 2)
	if len(gray.Pix) != 2*2 {
		t.Fatalf("pixel count = %d, want 4", len(gray.Pix))
	}
}

func TestResolveModel(t *testing.T) {
	m, ok := ResolveModel("depth-anything-v2-small")
	if !ok || m.Filename == "" {
		t.Fatalf("ResolveModel(full) = %v, %v; want valid model", m, ok)
	}
	if m.License != "Apache-2.0" {
		t.Errorf("small license = %q, want Apache-2.0", m.License)
	}
	// Alias support: "small" resolves to the same model.
	alias, ok := ResolveModel("small")
	if !ok || alias.ID != "depth-anything-v2-small" {
		t.Fatalf("ResolveModel(alias small) = %v, %v", alias, ok)
	}
	if _, ok := ResolveModel("bogus"); ok {
		t.Fatal("ResolveModel(bogus) should return ok=false")
	}
	// ModelPath falls back to default for unknown IDs.
	if got := ModelPath("/models", "bogus"); got != "/models/depth/depth-anything-v2-small.onnx" {
		t.Errorf("ModelPath fallback = %q", got)
	}
}

func TestListModelIDs(t *testing.T) {
	ids := ListModelIDs()
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("ListModelIDs has duplicate %q (got %v)", id, ids)
		}
		seen[id] = true
	}
	for _, want := range []string{
		"depth-anything-v2-small", "depth-anything-v2-base", "depth-anything-v2-large",
	} {
		if !seen[want] {
			t.Errorf("ListModelIDs missing %s (got %v)", want, ids)
		}
	}
	if ids[0] != DefaultModelID {
		t.Errorf("ListModelIDs[0] = %q, want default %q", ids[0], DefaultModelID)
	}
}

func TestPatchAlignment(t *testing.T) {
	// 518 must be divisible by patch size 14 (518 = 37*14).
	if ModelInputSize%ModelPatchSize != 0 {
		t.Errorf("%d %% %d != 0", ModelInputSize, ModelPatchSize)
	}
	if math.Abs(float64(ModelInputSize/ModelPatchSize)-37) > 0 {
		t.Logf("patches = %d", ModelInputSize/ModelPatchSize)
	}
}
