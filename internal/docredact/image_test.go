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
	blackPixels := 0
	for x := 0; x < 200; x++ {
		r, g, b, _ := got.At(x, 20).RGBA()
		if r < 0x3000 && g < 0x3000 && b < 0x3000 {
			blackPixels++
		}
	}
	if blackPixels == 0 {
		t.Fatal("expected at least one black box")
	}
	if blackPixels > 160 {
		t.Fatalf("black box covered almost the whole line: %d/200 pixels", blackPixels)
	}
	r, g, b, _ := got.At(5, 20).RGBA()
	if r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Fatalf("leading background should stay white, got rgb=(%d,%d,%d)", r>>8, g>>8, b>>8)
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

func TestImageCrossLineSpan(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.png")
	img := image.NewRGBA(image.Rect(0, 0, 400, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 400; x++ {
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
	mock := mockOCR{result: &ocr.OCRResult{Pages: []ocr.OCRPage{{Lines: []ocr.OCRLine{
		{Text: "身份证号32010419870312", BBox: [4][2]int{{10, 10}, {390, 10}, {390, 45}, {10, 45}}},
		{Text: "5517，电话13812345678。", BBox: [4][2]int{{10, 55}, {390, 55}, {390, 90}, {10, 90}}},
	}}}}}
	out := filepath.Join(dir, "out.jpg")
	res, err := Apply(context.Background(), in, out, pipeline, Options{Style: "mask", OCR: mock})
	if err != nil {
		t.Fatal(err)
	}
	foundID, foundPhone := false, false
	for _, span := range res.Spans {
		if span.Type == "ID_CARD" {
			foundID = true
		}
		if span.Type == "PHONE" {
			foundPhone = true
		}
	}
	if !foundID || !foundPhone {
		t.Fatalf("cross-line spans not detected: %+v", res.Spans)
	}
}
