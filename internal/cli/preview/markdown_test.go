package preview

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestIsMarkdownPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"notes.md", true},
		{"NOTES.MD", true},
		{"notes.markdown", true},
		{"notes.mdown", true},
		{filepath.Join("dir", "sub", "scan.md"), true},
		{"notes.txt", false},
		{"notes.md.png", false},
		{"notes", false},
		{"https://example.com/page.md", false},
		{"http://example.com/page.markdown", false},
	}
	for _, tt := range tests {
		if got := isMarkdownPath(tt.path); got != tt.want {
			t.Errorf("isMarkdownPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func writeMarkdownFile(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	return path
}

func captureOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	t.Cleanup(restore)
	return &buf
}

func TestPreviewMarkdownKeepsSourceOutsideTerminal(t *testing.T) {
	src := "# Title\n\nbody without trailing newline"
	path := writeMarkdownFile(t, src)
	buf := captureOutput(t)

	if err := previewMarkdown(path, false, false); err != nil {
		t.Fatalf("previewMarkdown: %v", err)
	}
	if got, want := buf.String(), src+"\n"; got != want {
		t.Errorf("raw output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestPreviewMarkdownRendersInTerminal(t *testing.T) {
	oldAllowed := markdownRenderAllowed
	markdownRenderAllowed = func(raw bool) bool { return !raw }
	t.Cleanup(func() { markdownRenderAllowed = oldAllowed })

	src := "# Title\n\n- one\n- two\n"
	path := writeMarkdownFile(t, src)
	buf := captureOutput(t)

	if err := previewMarkdown(path, false, false); err != nil {
		t.Fatalf("previewMarkdown: %v", err)
	}
	rendered := buf.String()
	if !strings.Contains(rendered, "Title") {
		t.Errorf("rendered output lost the heading text: %q", rendered)
	}
	if strings.Contains(rendered, "# Title") {
		t.Errorf("rendered output kept the heading marker: %q", rendered)
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Errorf("rendered output has no ANSI styling: %q", rendered)
	}

	// --raw must win over the terminal probe.
	buf.Reset()
	if err := previewMarkdown(path, true, false); err != nil {
		t.Fatalf("previewMarkdown raw: %v", err)
	}
	if got := buf.String(); got != src {
		t.Errorf("raw flag output mismatch\n got: %q\nwant: %q", got, src)
	}
}

func TestPreviewMarkdownHonorsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	oldAllowed := markdownRenderAllowed
	markdownRenderAllowed = func(raw bool) bool { return !raw }
	t.Cleanup(func() { markdownRenderAllowed = oldAllowed })

	path := writeMarkdownFile(t, "# Title\n\nplain text\n")
	buf := captureOutput(t)

	if err := previewMarkdown(path, false, false); err != nil {
		t.Fatalf("previewMarkdown: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("NO_COLOR output still contains escapes: %q", out)
	}
	if !strings.Contains(out, "Title") {
		t.Errorf("NO_COLOR output lost the heading text: %q", out)
	}
}

func TestIsTextPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"notes.txt", true},
		{"NOTES.TXT", true},
		{"app.log", true},
		{"notes.text", true},
		{"notes.md", false},
		{"notes.md.png", false},
		{"notes", false},
		{"https://example.com/a.txt", false},
	}
	for _, tt := range tests {
		if got := isTextPath(tt.path); got != tt.want {
			t.Errorf("isTextPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestPreviewTextPrintsRaw(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("plain text\nsecond line"), 0o644); err != nil {
		t.Fatal(err)
	}
	buf := captureOutput(t)

	if err := previewText(path, false); err != nil {
		t.Fatalf("previewText: %v", err)
	}
	if got, want := buf.String(), "plain text\nsecond line\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestPreviewMarkdownPagesLongOutput(t *testing.T) {
	oldAllowed := markdownRenderAllowed
	markdownRenderAllowed = func(raw bool) bool { return !raw }
	t.Cleanup(func() { markdownRenderAllowed = oldAllowed })
	stubPagerSize(t, 80, 3)

	var fed string
	stubPager(t, nil, func(_ []string, content string) (bool, error) {
		fed = content
		return true, nil
	})
	buf := captureOutput(t)

	src := "# Title\n\n" + strings.Repeat("- item\n", 10)
	path := writeMarkdownFile(t, src)
	if err := previewMarkdown(path, false, false); err != nil {
		t.Fatalf("previewMarkdown: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("long document was printed instead of paged: %q", buf.String())
	}
	if !strings.Contains(fed, "Title") {
		t.Errorf("pager input lost the rendered heading: %q", fed)
	}
	if strings.Contains(fed, "# Title") {
		t.Errorf("pager input kept the heading marker: %q", fed)
	}
}

func TestPreviewMarkdownWithImagesSkipsPager(t *testing.T) {
	oldAllowed := markdownRenderAllowed
	markdownRenderAllowed = func(raw bool) bool { return !raw }
	t.Cleanup(func() { markdownRenderAllowed = oldAllowed })
	stubPagerSize(t, 80, 3)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pic.png"), "png")
	stubInline(t, func(string) bool { return true })
	stubPager(t, nil, func([]string, string) (bool, error) {
		t.Error("pager ran for a document with inline images")
		return true, nil
	})
	captureOutput(t)

	src := "# 图集\n\n![图](pic.png)\n\n" + strings.Repeat("- item\n", 10)
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := previewMarkdown(path, false, false); err != nil {
		t.Fatalf("previewMarkdown: %v", err)
	}
}

func TestMarkdownWrapWidthFallback(t *testing.T) {
	buf := captureOutput(t)

	if got := markdownWrapWidth(); got != fallbackMarkdownWidth {
		t.Errorf("markdownWrapWidth() = %d, want fallback %d (buffer %q)", got, fallbackMarkdownWidth, buf)
	}
}
