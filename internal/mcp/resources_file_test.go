package mcp

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// readFileResource invokes the output-file template handler.
func readFileResource(t *testing.T, handler server.ResourceTemplateHandlerFunc, uri string) []mcp.ResourceContents {
	t.Helper()
	return readResource(t, server.ResourceHandlerFunc(handler), uri)
}

// readFileResourceErr requires the output-file template handler to fail.
func readFileResourceErr(t *testing.T, handler server.ResourceTemplateHandlerFunc, uri string) error {
	t.Helper()
	return readResourceErr(t, server.ResourceHandlerFunc(handler), uri)
}

func TestOutputFileResource_returnsBlobForImage(t *testing.T) {
	dir := t.TempDir()
	payload := "\x89PNG\r\n\x1a\nfake-image-data"
	writeFile(t, filepath.Join(dir, "pic.png"), payload)

	contents := readFileResource(t, outputFileHandler(&Config{Output: dir}), "aigc://output/pic.png")
	if len(contents) != 1 {
		t.Fatalf("contents count = %d, want 1", len(contents))
	}
	blob, ok := contents[0].(mcp.BlobResourceContents)
	if !ok {
		t.Fatalf("contents[0] type = %T, want BlobResourceContents", contents[0])
	}
	if blob.MIMEType != "image/png" {
		t.Errorf("MIMEType = %q, want image/png", blob.MIMEType)
	}
	decoded, err := base64.StdEncoding.DecodeString(blob.Blob)
	if err != nil {
		t.Fatalf("decode blob: %v", err)
	}
	if string(decoded) != payload {
		t.Errorf("blob payload = %q, want %q", decoded, payload)
	}
}

func TestOutputFileResource_returnsTextForTextFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "note.txt"), "hello resource")

	contents := readFileResource(t, outputFileHandler(&Config{Output: dir}), "aigc://output/note.txt")
	if len(contents) != 1 {
		t.Fatalf("contents count = %d, want 1", len(contents))
	}
	text, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("contents[0] type = %T, want TextResourceContents", contents[0])
	}
	if text.Text != "hello resource" {
		t.Errorf("Text = %q, want %q", text.Text, "hello resource")
	}
	if text.MIMEType != "text/plain" {
		t.Errorf("MIMEType = %q, want text/plain", text.MIMEType)
	}
}

func TestOutputFileResource_rejectsUnsafeNames(t *testing.T) {
	handler := outputFileHandler(&Config{Output: t.TempDir()})
	cases := []struct {
		label string
		uri   string
	}{
		{"parent", outputURIPrefix + "../secret"},
		{"nested", outputURIPrefix + "a/b"},
		{"absolute", outputURIPrefix + "/etc/passwd"},
		{"windows-separator", outputURIPrefix + `..\x`},
		{"empty", outputURIPrefix},
		{"volume", outputURIPrefix + "C:secret.txt"},
		{"outside-uri", "aigc://elsewhere/note.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			readFileResourceErr(t, handler, tc.uri)
		})
	}
}

func TestResolveOutputPath_rejectsFileOutsideRoot(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.png")
	writeFile(t, outside, "secret")

	if _, err := resolveOutputPath(t.TempDir(), outside); err == nil {
		t.Fatal("expected an error for a file outside the output directory")
	}
}

func TestResolveOutputPath_acceptsPlainName(t *testing.T) {
	root := t.TempDir()
	got, err := resolveOutputPath(root, "img.png")
	if err != nil {
		t.Fatalf("resolveOutputPath: %v", err)
	}
	if want := filepath.Join(root, "img.png"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOutputFileResource_rejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "big.txt"), strings.Repeat("x", maxResourceFileSize+1))

	err := readFileResourceErr(t, outputFileHandler(&Config{Output: dir}), "aigc://output/big.txt")
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %v, want a 'too large' rejection", err)
	}
}

func TestOutputFileResource_rejectsUnsupportedType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "archive.zip"), "zip")

	err := readFileResourceErr(t, outputFileHandler(&Config{Output: dir}), "aigc://output/archive.zip")
	if !strings.Contains(err.Error(), "unsupported resource type") {
		t.Errorf("error = %v, want an unsupported-type rejection", err)
	}
}

func TestOutputFileResource_rejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, target, "secret")
	if err := os.Symlink(target, filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := readFileResourceErr(t, outputFileHandler(&Config{Output: dir}), "aigc://output/link.txt")
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %v, want a symlink rejection", err)
	}
}

func TestOutputFileResource_missingFile(t *testing.T) {
	err := readFileResourceErr(t, outputFileHandler(&Config{Output: t.TempDir()}), "aigc://output/nope.txt")
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want a not-found rejection", err)
	}
}

func TestOutputFileResource_noOutputDirectory(t *testing.T) {
	err := readFileResourceErr(t, outputFileHandler(&Config{}), "aigc://output/note.txt")
	if !strings.Contains(err.Error(), "no output directory") {
		t.Errorf("error = %v, want a no-output-directory rejection", err)
	}
}
