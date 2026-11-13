package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownImageRefs(t *testing.T) {
	src := strings.Join([]string{
		"# Doc",
		"",
		"![sunset](images/sunset.png \"Sunset\")",
		"",
		`<img src="pics/raw.png" alt="raw">`,
		"",
		"```md",
		"![code](code.png)",
		"```",
		"",
		"[![linked](linked.png)](https://example.com)",
		"",
		"![ref][fig]",
		"",
		"[fig]: ./images/ref.png",
	}, "\n")

	refs := markdownImageRefs(src)
	if len(refs) != 3 {
		t.Fatalf("refs = %d (%+v), want 3", len(refs), refs)
	}

	want := []struct{ alt, target string }{
		{"sunset", "images/sunset.png"},
		{"raw", "pics/raw.png"},
		{"ref", "./images/ref.png"},
	}
	for i, w := range want {
		if refs[i].alt != w.alt || refs[i].target != w.target {
			t.Errorf("ref %d = %+v, want alt=%q target=%q", i, refs[i], w.alt, w.target)
		}
		if got := src[refs[i].start:refs[i].end]; !strings.Contains(got, w.target) && !strings.Contains(got, "![ref]") {
			t.Errorf("ref %d span = %q, want it to cover the image source", i, got)
		}
	}
}

func TestResolveLocalMarkdownImage(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "images", "a b.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		target string
		want   string
		ok     bool
	}{
		{"relative", "images/a%20b.png", imagePath, true},
		{"dot relative", "./images/a b.png", imagePath, true},
		{"absolute", imagePath, imagePath, true},
		{"file url", "file://" + imagePath, imagePath, true},
		{"missing", "images/missing.png", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, cleanup, ok := resolveMarkdownImage(tc.target, dir)
			if cleanup != nil {
				cleanup()
			}
			if ok != tc.ok {
				t.Fatalf("resolveMarkdownImage(%q) ok = %v, want %v", tc.target, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Errorf("resolveMarkdownImage(%q) = %q, want %q", tc.target, got, tc.want)
			}
		})
	}
}

func TestRenderMarkdownDocumentInlinesImages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pic.png"), "png")
	src := "# 图集\n\n前文\n\n![日落](pic.png)\n\n后文\n"

	var shown []string
	stubInline(t, func(path string) bool {
		shown = append(shown, path)
		return true
	})
	buf := captureOutput(t)

	renderMarkdownWithImages(src, dir, 80)

	if len(shown) != 1 || filepath.Base(shown[0]) != "pic.png" {
		t.Fatalf("inline previews = %v, want pic.png", shown)
	}
	out := buf.String()
	if !strings.Contains(out, "图集") || !strings.Contains(out, "前文") || !strings.Contains(out, "后文") {
		t.Errorf("text around the image was lost:\n%s", out)
	}
	if !strings.Contains(out, "  日落") {
		t.Errorf("image caption missing:\n%s", out)
	}
	if strings.Contains(out, "pic.png") {
		t.Errorf("image reference was left in the rendered text:\n%s", out)
	}
}

func TestRenderMarkdownDocumentImageWithoutAltHasNoCaption(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pic.png"), "png")

	var shown []string
	stubInline(t, func(path string) bool {
		shown = append(shown, path)
		return true
	})
	buf := captureOutput(t)

	renderMarkdownWithImages("![](pic.png)\n", dir, 80)

	if len(shown) != 1 {
		t.Fatalf("inline previews = %v, want one", shown)
	}
	if out := buf.String(); strings.TrimSpace(out) != "" {
		t.Errorf("image without alt text printed a caption: %q", out)
	}
}

func TestRenderMarkdownDocumentKeepsReferenceWhenInlineUnavailable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pic.png"), "png")

	called := false
	stubInline(t, func(string) bool {
		called = true
		return true
	})
	oldAvailable := inlinePreviewAvailable
	inlinePreviewAvailable = func() bool { return false }
	t.Cleanup(func() { inlinePreviewAvailable = oldAvailable })

	buf := captureOutput(t)
	renderMarkdownWithImages("![日落](pic.png)\n", dir, 80)

	if called {
		t.Errorf("inline preview was attempted without terminal support")
	}
	if out := buf.String(); !strings.Contains(out, "日落") {
		t.Errorf("image reference disappeared without inline support:\n%s", out)
	}
}

func TestRenderMarkdownDocumentDataURI(t *testing.T) {
	// 1x1 transparent PNG.
	const tinyPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

	var shown string
	stubInline(t, func(path string) bool {
		shown = path
		return true
	})
	captureOutput(t)

	renderMarkdownWithImages("![dot]("+tinyPNG+")\n", t.TempDir(), 80)

	if shown == "" {
		t.Fatal("data URI image was not previewed")
	}
	if ext := filepath.Ext(shown); ext != ".png" {
		t.Errorf("temp image extension = %q, want .png", ext)
	}
	if _, err := os.Stat(shown); !os.IsNotExist(err) {
		t.Errorf("temp image %q was not cleaned up", shown)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// stubInline replaces the inline-image hooks for one test.
func stubInline(t *testing.T, fn func(string) bool) {
	t.Helper()
	oldAvailable := inlinePreviewAvailable
	oldFile := inlinePreviewFile
	inlinePreviewAvailable = func() bool { return true }
	inlinePreviewFile = fn
	t.Cleanup(func() {
		inlinePreviewAvailable = oldAvailable
		inlinePreviewFile = oldFile
	})
}
