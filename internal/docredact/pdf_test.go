package docredact

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gopdf "github.com/razvandimescu/gopdf/pdf"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

func TestPDFRasterizedOutputHasNoText(t *testing.T) {
	if _, err := exec.LookPath("mutool"); err != nil {
		t.Skip("mutool not installed")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pdf")
	creator := gopdf.NewCreator()
	page := creator.NewPage(300, 200)
	page.SetFont("Helvetica", 12)
	page.DrawText(20, 150, "Email: a@example.com phone 13812345678")
	data, err := creator.Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, data, 0o600); err != nil {
		t.Fatal(err)
	}

	engine, _ := redactlib.NewRuleEngine(nil)
	pipeline := redactlib.NewPipeline(engine)
	out := filepath.Join(dir, "out.pdf")
	res, err := Apply(context.Background(), in, out, pipeline, Options{DPI: 150})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Spans) < 2 {
		t.Fatalf("spans = %+v", res.Spans)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("%PDF-")) {
		t.Fatalf("output is not a PDF")
	}
	if bytes.Contains(raw, []byte("a@example.com")) || bytes.Contains(raw, []byte("13812345678")) {
		t.Fatalf("output PDF still contains original text")
	}
	if !strings.Contains(string(raw), "/DCTDecode") {
		t.Fatalf("output PDF pages are not JPEG images")
	}
}
