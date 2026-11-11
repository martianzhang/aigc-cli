package redact

import (
	"archive/zip"
	"bytes"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gopdf "github.com/razvandimescu/gopdf/pdf"
)

func writeTestDOCX(t *testing.T, path, text string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + html.EscapeString(text) + `</w:t></w:r></w:p></w:body></w:document>`,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestDOCX(t *testing.T, path string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
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
	}
	t.Fatal("word/document.xml missing")
	return ""
}

func TestDOCXApplyAndRestoreViaCLI(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.docx")
	writeTestDOCX(t, src, "原告：张伟，电话13812345678。")
	mapping := filepath.Join(dir, "in.map.json")

	if _, err := runCommand(t, Deps{OutputDir: dir}, "apply", src, "--style", "placeholder", "--mapping", mapping); err != nil {
		t.Fatal(err)
	}
	redacted := filepath.Join(dir, "in.redacted.docx")
	xml := readTestDOCX(t, redacted)
	if !strings.Contains(xml, "&lt;PERSON_1&gt;") || !strings.Contains(xml, "&lt;PHONE_1&gt;") {
		t.Fatalf("docx not redacted:\n%s", xml)
	}

	if _, err := runCommand(t, Deps{OutputDir: dir}, "check", redacted, "--mapping", mapping); err != nil {
		t.Fatalf("check clean docx: %v", err)
	}
	if _, err := runCommand(t, Deps{OutputDir: dir}, "restore", redacted, "--mapping", mapping); err != nil {
		t.Fatal(err)
	}
	restored := readTestDOCX(t, filepath.Join(dir, "in.restored.docx"))
	if !strings.Contains(restored, "张伟") || !strings.Contains(restored, "13812345678") {
		t.Fatalf("restore failed:\n%s", restored)
	}
}

func TestPDFScanAndApplyViaCLI(t *testing.T) {
	if _, err := exec.LookPath("mutool"); err != nil {
		t.Skip("mutool not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "in.pdf")
	creator := gopdf.NewCreator()
	page := creator.NewPage(300, 200)
	page.SetFont("Helvetica", 12)
	page.DrawText(20, 150, "Email: a@example.com phone 13812345678")
	data, err := creator.Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runCommand(t, Deps{OutputDir: dir}, "scan", src, "--dpi", "100")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "EMAIL") || !strings.Contains(out, "PHONE") {
		t.Fatalf("scan output = %s", out)
	}
	if _, err := runCommand(t, Deps{OutputDir: dir}, "apply", src, "--dpi", "100"); err != nil {
		t.Fatal(err)
	}
	redacted := filepath.Join(dir, "in.redacted.pdf")
	raw, err := os.ReadFile(redacted)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("a@example.com")) || bytes.Contains(raw, []byte("13812345678")) {
		t.Fatal("redacted PDF still contains original text")
	}
	if !bytes.Contains(raw, []byte("/DCTDecode")) {
		t.Fatal("redacted PDF is not image-only")
	}
}
