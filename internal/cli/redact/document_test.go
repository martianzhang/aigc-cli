package redact

import (
	"archive/zip"
	"bytes"
	"html"
	"image"
	"image/color"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestLegacyDOCRejectedViaCLI(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "old.doc")
	if err := os.WriteFile(src, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runCommand(t, Deps{OutputDir: dir}, "apply", src)
	if err == nil || !strings.Contains(err.Error(), "legacy .doc") {
		t.Fatalf("error = %v, want legacy .doc guidance", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "old.redacted.doc")); statErr == nil {
		t.Fatal(".doc was treated as text and produced an output file")
	}
}

func TestEncryptedPDFRejectedViaCLI(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "enc.pdf")
	body := "%PDF-1.7\n1 0 obj<<>>endobj\ntrailer<< /Encrypt 5 0 R >>\n%%EOF\n"
	if err := os.WriteFile(src, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runCommand(t, Deps{OutputDir: dir}, "scan", src)
	if err == nil || !strings.Contains(err.Error(), "encrypted") {
		t.Fatalf("error = %v, want encrypted PDF guidance", err)
	}
}

func TestAnimatedGIFRejectedViaCLI(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "anim.gif")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	pal := color.Palette{color.White, color.Black}
	frame1 := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	frame2 := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	if err := gif.EncodeAll(f, &gif.GIF{Image: []*image.Paletted{frame1, frame2}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	f.Close()
	_, err = runCommand(t, Deps{OutputDir: dir}, "apply", src)
	if err == nil || !strings.Contains(err.Error(), "animated GIF") {
		t.Fatalf("error = %v, want animated GIF rejection", err)
	}
}

func TestLegacyDOCConversionViaCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake soffice script is POSIX-only")
	}
	dir := t.TempDir()
	fixture := filepath.Join(dir, "fixture.docx")
	writeTestDOCX(t, fixture, "原告：张伟，电话13812345678。")
	src := filepath.Join(dir, "old.doc")
	if err := os.WriteFile(src, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(bin, "soffice")
	body := "#!/bin/sh\noutdir=\"\"\nwhile [ $# -gt 0 ]; do\n  case \"$1\" in\n    --outdir) outdir=\"$2\"; shift 2;;\n    *) src=\"$1\"; shift;;\n  esac\ndone\nbase=$(basename \"$src\" .doc)\ncp \"$FIXTURE_DOCX\" \"$outdir/$base.docx\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIXTURE_DOCX", fixture)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := runCommand(t, Deps{OutputDir: dir}, "apply", src, "--style", "mask"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "old.redacted.docx")
	xml := readTestDOCX(t, out)
	if !strings.Contains(xml, "张*") || !strings.Contains(xml, "138****5678") {
		t.Fatalf("converted output not masked:\n%s", xml)
	}
}
