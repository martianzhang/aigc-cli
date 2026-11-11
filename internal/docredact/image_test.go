package docredact

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ocr"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

type mockOCR struct{ result *ocr.OCRResult }

func (m mockOCR) Scan(image.Image) (*ocr.OCRResult, error) { return m.result, nil }

func TestImageBlackBoxes(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.png")
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.White)
		}
	}
	f, err := os.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	engine, _ := redactlib.NewRuleEngine(nil)
	pipeline := redactlib.NewPipeline(engine)
	mock := mockOCR{result: &ocr.OCRResult{Pages: []ocr.OCRPage{{Lines: []ocr.OCRLine{{
		Text: "原告：张伟，电话13812345678。",
		BBox: [4][2]int{{10, 10}, {190, 10}, {190, 40}, {10, 40}},
	}}}}}}
	out := filepath.Join(dir, "out.jpg")
	res, err := Apply(context.Background(), in, out, pipeline, Options{Style: "mask", OCR: mock})
	if err != nil {
		t.Fatal(err)
	}
	if res.OutputPath != out {
		t.Fatalf("output = %q, want %q", res.OutputPath, out)
	}
	if len(res.Spans) < 2 {
		t.Fatalf("spans = %+v", res.Spans)
	}
	f, err = os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := got.At(20, 20).RGBA()
	if r > 0x3000 || g > 0x3000 || b > 0x3000 {
		t.Fatalf("expected black box, got rgb=(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

func TestTransparentImageBecomesJPG(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.png")
	img := image.NewNRGBA(image.Rect(0, 0, 100, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 100; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 0})
		}
	}
	f, err := os.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	engine, _ := redactlib.NewRuleEngine(nil)
	pipeline := redactlib.NewPipeline(engine)
	mock := mockOCR{result: &ocr.OCRResult{Pages: []ocr.OCRPage{{Lines: []ocr.OCRLine{{
		Text: "电话13812345678",
		BBox: [4][2]int{{5, 5}, {95, 5}, {95, 40}, {5, 40}},
	}}}}}}
	out := filepath.Join(dir, "out.png")
	res, err := Apply(context.Background(), in, out, pipeline, Options{Style: "mask", OCR: mock})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(res.OutputPath, ".jpg") {
		t.Fatalf("output = %q, want .jpg", res.OutputPath)
	}
	if _, err := os.Stat(res.OutputPath); err != nil {
		t.Fatal(err)
	}
}
