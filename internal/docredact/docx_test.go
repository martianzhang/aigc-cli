package docredact

import (
	"archive/zip"
	"bytes"
	"context"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

func makeDocx(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.docx")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	write("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`)
	write("word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>`+html.EscapeString(text)+`</w:t></w:r></w:p></w:body></w:document>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func docxText(t *testing.T, path string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		var b bytes.Buffer
		if _, err := b.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	t.Fatal("word/document.xml missing")
	return ""
}

func TestDOCXMaskAndRestore(t *testing.T) {
	in := makeDocx(t, "原告：张伟，电话13812345678。")
	engine, err := redactlib.NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	pipeline := redactlib.NewPipeline(engine)
	masks, _ := redactlib.NewMaskResolver(nil)
	out := filepath.Join(t.TempDir(), "out.docx")
	res, err := Apply(context.Background(), in, out, pipeline, Options{Style: "mask", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	xml := docxText(t, out)
	if !strings.Contains(xml, "张*") || !strings.Contains(xml, "138****5678") {
		t.Fatalf("docx not masked:\n%s", xml)
	}
	if res.Mapping != nil {
		t.Fatalf("mask should not produce a mapping")
	}

	in2 := makeDocx(t, "原告：张伟，电话13812345678。")
	out2 := filepath.Join(t.TempDir(), "out2.docx")
	res2, err := Apply(context.Background(), in2, out2, pipeline, Options{Style: "placeholder", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Mapping == nil {
		t.Fatal("placeholder should produce a mapping")
	}
	restored := filepath.Join(t.TempDir(), "restored.docx")
	if err := RestoreDOCX(out2, restored, res2.Mapping); err != nil {
		t.Fatal(err)
	}
	if xml := docxText(t, restored); !strings.Contains(xml, "张伟") || !strings.Contains(xml, "13812345678") {
		t.Fatalf("restore failed:\n%s", xml)
	}
}
