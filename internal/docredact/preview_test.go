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

func TestPreviewTextDOCXMatchesFileOutput(t *testing.T) {
	in := makeDocx(t, "原告：张伟，电话13812345678。")
	engine, err := redactlib.NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	pipeline := redactlib.NewPipeline(engine)
	masks, err := redactlib.NewMaskResolver(nil)
	if err != nil {
		t.Fatal(err)
	}

	preview, err := PreviewText(context.Background(), in, pipeline, Options{Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.Text, "<PERSON_1>") || !strings.Contains(preview.Text, "<PHONE_1>") {
		t.Fatalf("preview text = %q", preview.Text)
	}
	if preview.Mapping == nil {
		t.Fatal("preview mapping is nil")
	}
	if preview.Mapping.OutputSHA256 == "" {
		t.Fatal("preview mapping has no output hash")
	}
	// The printed text must be restorable: restore checks the mapping hash.
	restored, err := redactlib.Restore(preview.Text, preview.Mapping, false)
	if err != nil {
		t.Fatalf("restore preview: %v", err)
	}
	if !strings.Contains(restored, "张伟") || !strings.Contains(restored, "13812345678") {
		t.Fatalf("restored = %q", restored)
	}

	// The file output must contain the same redacted text.
	out := filepath.Join(t.TempDir(), "out.docx")
	if _, err := Apply(context.Background(), in, out, pipeline, Options{Masks: masks}); err != nil {
		t.Fatal(err)
	}
	text, err := ExtractText(out)
	if err != nil {
		t.Fatal(err)
	}
	if text != preview.Text {
		t.Fatalf("docx text = %q, preview = %q", text, preview.Text)
	}
}

func TestPreviewTextImageUsesStyle(t *testing.T) {
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

	engine, err := redactlib.NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	pipeline := redactlib.NewPipeline(engine)
	masks, err := redactlib.NewMaskResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	mock := mockOCR{result: &ocr.OCRResult{Pages: []ocr.OCRPage{{Lines: []ocr.OCRLine{{
		Text: "原告：张伟，电话13812345678。",
		BBox: [4][2]int{{10, 10}, {190, 10}, {190, 40}, {10, 40}},
	}}}}}}

	res, err := PreviewText(context.Background(), in, pipeline, Options{Style: "mask", Masks: masks, OCR: mock})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "138****5678") || !strings.Contains(res.Text, "张*") {
		t.Fatalf("preview text = %q", res.Text)
	}
	if strings.Contains(res.Text, "13812345678") {
		t.Fatalf("preview text leaked the original value: %q", res.Text)
	}
	if res.Mapping == nil {
		t.Fatal("preview mapping is nil")
	}
}
