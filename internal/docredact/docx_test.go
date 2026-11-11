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

func TestDOCXFootnotesEndnotesAndComments(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.docx")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, text string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(`<?xml version="1.0"?><w:root xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:r><w:t>` + html.EscapeString(text) + `</w:t></w:r></w:p></w:root>`)); err != nil {
			t.Fatal(err)
		}
	}
	write("word/document.xml", "原告：张伟，电话13812345678。")
	write("word/footnotes.xml", "脚注：原告李四，电话13900001111。")
	write("word/endnotes.xml", "尾注：原告王五，电话13700002222。")
	write("word/comments.xml", "批注：原告赵六，电话13600003333。")
	write("word/glossary/document.xml", "术语：原告钱七，电话13500004444。")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	engine, _ := redactlib.NewRuleEngine(nil)
	pipeline := redactlib.NewPipeline(engine)
	masks, _ := redactlib.NewMaskResolver(nil)
	out := filepath.Join(dir, "out.docx")
	if _, err := Apply(context.Background(), in, out, pipeline, Options{Style: "mask", Masks: masks}); err != nil {
		t.Fatal(err)
	}

	for _, part := range []string{"word/document.xml", "word/footnotes.xml", "word/endnotes.xml", "word/comments.xml", "word/glossary/document.xml"} {
		xml := docxPartText(t, out, part)
		if !strings.Contains(xml, "张*") && !strings.Contains(xml, "李*") && !strings.Contains(xml, "王*") && !strings.Contains(xml, "赵*") && !strings.Contains(xml, "钱*") {
			t.Errorf("%s was not masked: %s", part, xml)
		}
		if strings.Contains(xml, "13812345678") || strings.Contains(xml, "13900001111") || strings.Contains(xml, "13700002222") || strings.Contains(xml, "13600003333") || strings.Contains(xml, "13500004444") {
			t.Errorf("%s still contains a phone: %s", part, xml)
		}
	}
}

func docxPartText(t *testing.T, path, part string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != part {
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
	t.Fatalf("%s missing", part)
	return ""
}

func TestEncryptedDOCXClearError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "encrypted.docx")
	if err := os.WriteFile(in, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(context.Background(), in, filepath.Join(dir, "out.docx"), nil, Options{})
	if err == nil || !strings.Contains(err.Error(), "encrypted") {
		t.Fatalf("error = %v, want encrypted/legacy message", err)
	}
}

func TestDOCXEmbeddedObjectRejected(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "embedded.docx")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	_, _ = w.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>电话13812345678</w:t></w:r></w:p></w:body></w:document>`))
	w, _ = zw.Create("word/embeddings/oleObject1.bin")
	_, _ = w.Write([]byte("binary"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(context.Background(), in, filepath.Join(dir, "out.docx"), nil, Options{})
	if err == nil || !strings.Contains(err.Error(), "embedded objects") {
		t.Fatalf("error = %v, want embedded object guidance", err)
	}
}
