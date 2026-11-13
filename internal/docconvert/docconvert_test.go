package docconvert

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gopdf "github.com/razvandimescu/gopdf/pdf"
)

func TestSupported(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"paper.pdf", true},
		{"PAPER.PDF", true},
		{"book.epub", true},
		{"README.docx", true},
		{"page.html", true},
		{"page.htm", true},
		{"page.md", false},
		{"page.txt", false},
		{"page", false},
	}
	for _, tt := range tests {
		if got := Supported(tt.path); got != tt.want {
			t.Errorf("Supported(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestMarkdownRejectsUnsupportedFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("plain"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Markdown(path); err == nil {
		t.Fatal("Markdown(txt) succeeded, want an error")
	}
}

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	archive := zip.NewWriter(file)
	for name, content := range entries {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
}

func TestFromHTML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	html := `<!doctype html><html><head><title>Doc Title</title></head>
<body><h1>Heading</h1><p>Hello document body.</p></body></html>`
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}

	markdown, err := Markdown(path)
	if err != nil {
		t.Fatalf("Markdown(html): %v", err)
	}
	for _, want := range []string{"Heading", "Hello document body"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown missing %q:\n%s", want, markdown)
		}
	}
}

func TestFromDOCX(t *testing.T) {
	const body = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
  <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Title</w:t></w:r></w:p>
  <w:p><w:r><w:t>Hello world</w:t></w:r></w:p>
  <w:p><w:pPr><w:numPr/></w:pPr><w:r><w:t>First item</w:t></w:r></w:p>
  <w:tbl>
    <w:tr><w:tc><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>B</w:t></w:r></w:p></w:tc></w:tr>
    <w:tr><w:tc><w:p><w:r><w:t>1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>2</w:t></w:r></w:p></w:tc></w:tr>
  </w:tbl>
</w:body></w:document>`
	path := filepath.Join(t.TempDir(), "doc.docx")
	writeZip(t, path, map[string]string{"word/document.xml": body})

	markdown, err := Markdown(path)
	if err != nil {
		t.Fatalf("Markdown(docx): %v", err)
	}
	wants := []string{
		"# Title",
		"Hello world",
		"- First item",
		"| A | B |",
		"| --- | --- |",
		"| 1 | 2 |",
	}
	for _, want := range wants {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown missing %q:\n%s", want, markdown)
		}
	}
}

func TestFromEPUB(t *testing.T) {
	entries := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles>
<rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
</rootfiles></container>`,
		"OEBPS/content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Book Title</dc:title></metadata>
  <manifest><item id="ch1" href="ch1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="ch1"/></spine>
</package>`,
		"OEBPS/ch1.xhtml": `<html><body><h1>Chapter One</h1><p>Hello epub body.</p></body></html>`,
	}
	path := filepath.Join(t.TempDir(), "book.epub")
	writeZip(t, path, entries)

	markdown, err := Markdown(path)
	if err != nil {
		t.Fatalf("Markdown(epub): %v", err)
	}
	for _, want := range []string{"Chapter One", "Hello epub body"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown missing %q:\n%s", want, markdown)
		}
	}
}

func writeTextPDF(t *testing.T, path, text string) {
	t.Helper()
	creator := gopdf.NewCreator()
	page := creator.NewPage(300, 200)
	page.SetFont("Helvetica", 12)
	if text != "" {
		page.DrawText(20, 150, text)
	}
	data, err := creator.Build()
	if err != nil {
		t.Fatalf("build pdf: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
}

func TestFromPDFTextLayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "text.pdf")
	writeTextPDF(t, path, "HelloPDF text layer")

	markdown, err := Markdown(path)
	if err != nil {
		t.Fatalf("Markdown(pdf): %v", err)
	}
	if !strings.Contains(markdown, "HelloPDF") {
		t.Errorf("markdown missing PDF text:\n%s", markdown)
	}
}

func writeTwoColumnPDF(t *testing.T, path string) {
	t.Helper()
	creator := gopdf.NewCreator()
	page := creator.NewPage(400, 300)
	page.SetFont("Helvetica", 12)
	for i, y := range []float64{250, 230, 210, 190} {
		page.DrawText(20, y, fmt.Sprintf("Left column line %d", i))
		page.DrawText(220, y, fmt.Sprintf("Right column line %d", i))
	}
	data, err := creator.Build()
	if err != nil {
		t.Fatalf("build two-column pdf: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write two-column pdf: %v", err)
	}
}

func TestFromPDFComplexLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "columns.pdf")
	writeTwoColumnPDF(t, path)

	if _, err := Markdown(path); !errors.Is(err, ErrComplexLayout) {
		t.Fatalf("Markdown(two-column pdf) error = %v, want ErrComplexLayout", err)
	}
}

func TestFromPDFWithoutTextLayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scan.pdf")
	writeTextPDF(t, path, "")

	if _, err := Markdown(path); err == nil {
		t.Fatal("Markdown(blank pdf) succeeded, want ErrNoTextLayer")
	} else if !errors.Is(err, ErrNoTextLayer) {
		t.Fatalf("error = %v, want ErrNoTextLayer", err)
	}
}

func TestSuspiciousText(t *testing.T) {
	clean := "This is a normal paragraph with enough words to pass the threshold. " +
		"It should not be flagged as unreadable text at all."
	if suspiciousText(clean) {
		t.Error("clean text was flagged")
	}

	if !suspiciousText(strings.Repeat("\ufffd", 120)) {
		t.Error("replacement characters were not flagged")
	}
}
