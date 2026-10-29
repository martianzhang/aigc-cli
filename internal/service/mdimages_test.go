package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pngBytes is a minimal 1x1 PNG.
var pngBytes = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
	0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
	0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
	0x00, 0x00, 0x03, 0x00, 0x01, 0x26, 0xE0, 0xFE,
	0x69, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
	0x44, 0xAE, 0x42, 0x60, 0x82,
}

func TestMarkdownImageSpans(t *testing.T) {
	markdown := strings.Join([]string{
		"# Title",
		"![figure](https://cdn.example.com/a.png)",
		"![angle](<https://cdn.example.com/b.jpg>)",
		`<img alt="x" src="https://cdn.example.com/c.png">`,
		`<img src='https://cdn.example.com/d.png'>`,
		"![local](./images/local.png)",
		"![append](./out/image_001.jpg)",
		"",
	}, "\n")

	spans := markdownImageSpans(markdown)
	if len(spans) != 4 {
		t.Fatalf("spans = %d, want 4 (local paths must be skipped)", len(spans))
	}
	want := []string{
		"https://cdn.example.com/a.png",
		"https://cdn.example.com/b.jpg",
		"https://cdn.example.com/c.png",
		"https://cdn.example.com/d.png",
	}
	for i, span := range spans {
		if span.url != want[i] {
			t.Errorf("span %d url = %q, want %q", i, span.url, want[i])
		}
		if markdown[span.start:span.end] != span.url {
			t.Errorf("span %d range does not cover the URL", i)
		}
	}
}

func TestLocalizeMarkdownImages(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.png", "/same.png":
			hits++
			w.Header().Set("Content-Type", "image/png")
			w.Write(pngBytes)
		case "/b.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("\xff\xd8\xff\xe0 fake jpeg"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	markdown := "# Doc\n\n" +
		"![a](" + srv.URL + "/a.png)\n" +
		"<img src=\"" + srv.URL + "/b.jpg\">\n" +
		"![dup](" + srv.URL + "/a.png)\n" +
		"![gone](" + srv.URL + "/missing.png)\n" +
		"![local](./images/local.png)\n"

	assetDir := filepath.Join(t.TempDir(), "doc.assets")
	rewritten, result := LocalizeMarkdownImages(markdown, assetDir, "doc.assets")

	if len(result.Saved) != 2 {
		t.Fatalf("saved = %v, want 2 (deduplicated)", result.Saved)
	}
	if len(result.Failed) != 1 || !strings.Contains(result.Failed[0], "/missing.png") {
		t.Errorf("failed = %v, want the 404 reported", result.Failed)
	}
	if hits != 1 {
		t.Errorf("a.png downloads = %d, want 1 (the duplicate link must reuse the file)", hits)
	}

	for _, name := range []string{"image_001.png", "image_002.jpg"} {
		if _, err := os.Stat(filepath.Join(assetDir, name)); err != nil {
			t.Errorf("expected %s: %v", name, err)
		}
	}
	if strings.Contains(rewritten, srv.URL+"/a.png") {
		t.Error("downloaded image links must be rewritten to local paths")
	}
	if !strings.Contains(rewritten, "doc.assets/image_001.png") {
		t.Errorf("rewritten markdown = %s", rewritten)
	}
	if !strings.Contains(rewritten, srv.URL+"/missing.png") {
		t.Error("failed downloads must keep the remote URL")
	}
	if !strings.Contains(rewritten, "./images/local.png") {
		t.Error("local links must be untouched")
	}
}

func TestImageExtension(t *testing.T) {
	// Sniffed format wins over a misleading URL suffix.
	if got := imageExtension(pngBytes, "https://x/a.webp"); got != ".png" {
		t.Errorf("imageExtension(png, .webp) = %q, want .png", got)
	}
	// Unknown bytes fall back to the URL suffix.
	if got := imageExtension([]byte("not-an-image"), "https://x/a.JPEG?token=1"); got != ".jpeg" {
		t.Errorf("imageExtension(unknown, .JPEG) = %q, want .jpeg", got)
	}
	// Nothing usable falls back to .png.
	if got := imageExtension([]byte("x"), "https://x/a"); got != ".png" {
		t.Errorf("imageExtension(unknown, no ext) = %q, want .png", got)
	}
}

func TestLocalizeMarkdownImages_NoImages(t *testing.T) {
	dir := t.TempDir()
	assetDir := filepath.Join(dir, "doc.assets")
	rewritten, result := LocalizeMarkdownImages("# no images here", assetDir, "doc.assets")
	if rewritten != "# no images here" {
		t.Errorf("markdown changed: %q", rewritten)
	}
	if len(result.Saved) != 0 || len(result.Failed) != 0 {
		t.Errorf("result = %+v, want empty", result)
	}
	if _, err := os.Stat(assetDir); err == nil {
		t.Error("asset dir must not be created when there is nothing to download")
	}
}

func TestLocalizeMarkdownImages_RejectsNonImageContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html>expired</html>")
	}))
	defer srv.Close()

	markdown := "![x](" + srv.URL + "/expired.png)"
	assetDir := filepath.Join(t.TempDir(), "doc.assets")
	rewritten, result := LocalizeMarkdownImages(markdown, assetDir, "doc.assets")
	if len(result.Saved) != 0 || len(result.Failed) != 1 {
		t.Fatalf("result = %+v, want one failure", result)
	}
	if rewritten != markdown {
		t.Errorf("markdown = %q, want it unchanged", rewritten)
	}
}
