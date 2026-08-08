package face

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	pigo "github.com/esimov/pigo/core"
)

func testModelsDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	return filepath.Join(home, ".config", "aigc-cli", "models")
}

func skipIfNoModels(t *testing.T, modelsDir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(modelsDir, "face", "facefinder")); err != nil {
		t.Skip("face cascades not installed; run `aigc-cli depth init --face`")
	}
}

func TestBoxDecode(t *testing.T) {
	det := pigo.Detection{Row: 100, Col: 200, Scale: 80, Q: 10.0}
	box := decodeBox(det)
	want := [4]float32{160, 60, 240, 140}
	if box != want {
		t.Fatalf("box = %v, want %v", box, want)
	}
}

func TestDetectionParams(t *testing.T) {
	// shiftFactor=0.15 会在大图上漏检（窗口步长过大错过脸中心），
	// 0.10 在召回与速度间平衡（1024x768 约 100ms）。防回归。
	if shiftFactor > 0.10 {
		t.Fatalf("shiftFactor = %v, want <= 0.10 (0.15 misses faces on large images)", shiftFactor)
	}
	if scaleFactor < 1.10 || scaleFactor > 1.20 {
		t.Fatalf("scaleFactor = %v, want in [1.10, 1.20]", scaleFactor)
	}
}

func TestNewDetector(t *testing.T) {
	modelsDir := testModelsDir(t)
	skipIfNoModels(t, modelsDir)
	det, err := NewDetector(modelsDir)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	if det.classifier == nil || det.puploc == nil {
		t.Fatal("detector internals not loaded")
	}
	if len(det.flpcs) != 9 {
		t.Fatalf("flpcs = %d cascades, want 9 (5 eye + 4 mouth)", len(det.flpcs))
	}
	if len(det.flpcs["lp84"]) != 2 {
		t.Fatalf("lp84 = %d cascades, want 2 (normal + nose flip)", len(det.flpcs["lp84"]))
	}
}

func TestDetectFaceSample(t *testing.T) {
	modelsDir := testModelsDir(t)
	skipIfNoModels(t, modelsDir)
	det, err := NewDetector(modelsDir)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	f, err := os.Open("testdata/sample.jpg")
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode sample: %v", err)
	}

	faces, err := det.Detect(img)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(faces) == 0 {
		t.Fatal("no face detected in sample.jpg")
	}

	face := faces[0]
	b := img.Bounds()
	for _, v := range face.Box {
		if v < 0 || v > float32(b.Dx()+b.Dy()) {
			t.Fatalf("box out of range: %v", face.Box)
		}
	}
	if face.Box[0] >= face.Box[2] || face.Box[1] >= face.Box[3] {
		t.Fatalf("invalid box: %v", face.Box)
	}
	if face.LeftEye == [2]float32{} || face.RightEye == [2]float32{} {
		t.Fatal("eyes not detected")
	}
	counted := 0
	for _, lm := range face.Landmarks {
		if lm != [2]float32{} {
			counted++
		}
	}
	if counted == 0 {
		t.Fatal("no landmarks detected")
	}
}

func TestDetectNoFace(t *testing.T) {
	modelsDir := testModelsDir(t)
	skipIfNoModels(t, modelsDir)
	det, err := NewDetector(modelsDir)
	if err != nil {
		t.Fatalf("NewDetector: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	faces, err := det.Detect(img)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(faces) != 0 {
		t.Fatalf("got %d faces on noise image, want 0", len(faces))
	}
}

func TestDrawLandmarkPoints(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	var lm [NumLandmarks][2]float32
	lm[0] = [2]float32{10, 20}
	lm[5] = [2]float32{50, 60}
	// 其余槽位零值（未检出），不应产生绘制。

	face := Face{
		Box:       [4]float32{5, 5, 60, 90},
		Score:     10,
		LeftEye:   [2]float32{15, 25},
		RightEye:  [2]float32{45, 25},
		Landmarks: lm,
	}
	Draw(img, []Face{face})

	// 零值槽位不产生绘制（该点区域保持透明）。
	_, _, _, a := img.At(0, 0).RGBA()
	if a != 0 {
		t.Error("pixel at (0,0) should remain transparent")
	}
	// 关键点绘制了：槽位 0 处应有蓝色点。
	c := color.RGBA{R: 0, G: 0, B: 255, A: 255}
	if got := img.At(10, 20); got != c {
		t.Errorf("landmark point = %v, want blue %v", got, c)
	}
	// 眼睛绘制了：左眼处应有绿色点。
	g := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	if got := img.At(15, 25); got != g {
		t.Errorf("left eye = %v, want green %v", got, g)
	}
	// 检测框绘制了：框边应有红色点。
	r := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	if got := img.At(5, 5); got != r {
		t.Errorf("box corner = %v, want red %v", got, r)
	}
}
