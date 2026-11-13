package preview

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/docconvert"
)

// stubStdoutTerminal pins the stdout TTY probe.
func stubStdoutTerminal(t *testing.T, isTTY bool) {
	t.Helper()
	old := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return isTTY }
	t.Cleanup(func() { stdoutIsTerminal = old })
}

func TestIsDocumentPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"paper.pdf", true},
		{"book.epub", true},
		{"doc.docx", true},
		{"page.html", true},
		{"page.htm", true},
		{"notes.md", false},
		{"notes.txt", false},
		{"https://example.com/paper.pdf", false},
	}
	for _, tt := range tests {
		if got := isDocumentPath(tt.path); got != tt.want {
			t.Errorf("isDocumentPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestPreviewDocumentHTML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	html := `<html><body><h1>Doc Heading</h1><p>Converted body text.</p></body></html>`
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := captureOutput(t)

	if err := previewDocument(path, commandOptions{}); err != nil {
		t.Fatalf("previewDocument: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Doc Heading") || !strings.Contains(out, "Converted body text") {
		t.Errorf("converted markdown missing:\n%s", out)
	}
	if strings.Contains(out, "<h1>") {
		t.Errorf("output still contains HTML:\n%s", out)
	}
}

func TestPreviewDocumentUnrepresentablePDFOpensViewer(t *testing.T) {
	for _, cause := range []error{docconvert.ErrComplexLayout, docconvert.ErrNoTextLayer} {
		t.Run(cause.Error(), func(t *testing.T) {
			oldMarkdown, oldViewer := documentMarkdown, previewSystemFile
			t.Cleanup(func() {
				documentMarkdown, previewSystemFile = oldMarkdown, oldViewer
			})
			documentMarkdown = func(string) (string, error) { return "", cause }
			opened := ""
			previewSystemFile = func(path string) error {
				opened = path
				return nil
			}
			stubStdoutTerminal(t, true)
			captureOutput(t)

			if err := previewDocument("paper.pdf", commandOptions{}); err != nil {
				t.Fatalf("previewDocument: %v", err)
			}
			if opened != "paper.pdf" {
				t.Errorf("system viewer opened %q, want paper.pdf", opened)
			}
		})
	}
}

func TestPreviewDocumentUnrepresentablePDFWithoutTerminal(t *testing.T) {
	oldMarkdown, oldViewer := documentMarkdown, previewSystemFile
	t.Cleanup(func() {
		documentMarkdown, previewSystemFile = oldMarkdown, oldViewer
	})
	documentMarkdown = func(string) (string, error) { return "", docconvert.ErrComplexLayout }
	opened := false
	previewSystemFile = func(string) error {
		opened = true
		return nil
	}
	stubStdoutTerminal(t, false)
	captureOutput(t)

	if err := previewDocument("paper.pdf", commandOptions{}); err == nil {
		t.Fatal("preview succeeded without a terminal")
	}
	if opened {
		t.Error("system viewer opened for non-terminal output")
	}
}

func TestPreviewDocumentConversionErrorSurfaces(t *testing.T) {
	oldMarkdown := documentMarkdown
	t.Cleanup(func() { documentMarkdown = oldMarkdown })
	documentMarkdown = func(string) (string, error) { return "", errors.New("broken docx") }
	captureOutput(t)

	if err := previewDocument("doc.docx", commandOptions{}); err == nil {
		t.Fatal("conversion error was swallowed")
	}
}
