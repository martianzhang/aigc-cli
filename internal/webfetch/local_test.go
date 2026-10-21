package webfetch

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestLocalFilePath(t *testing.T) {
	file := writeTempFile(t, "page.html", "<html></html>")
	dir := t.TempDir()

	if got, ok, err := LocalFilePath(file); err != nil || !ok || got != file {
		t.Errorf("LocalFilePath(file) = %q/%v/%v, want the file", got, ok, err)
	}

	fileURL := (&url.URL{Scheme: "file", Path: file}).String()
	if got, ok, err := LocalFilePath(fileURL); err != nil || !ok || got != file {
		t.Errorf("LocalFilePath(file URL) = %q/%v/%v, want the file", got, ok, err)
	}

	if _, ok, err := LocalFilePath("https://example.com/page"); err != nil || ok {
		t.Errorf("LocalFilePath(URL) = ok:%v err:%v, want not local", ok, err)
	}

	if _, _, err := LocalFilePath(dir); err == nil {
		t.Error("LocalFilePath(directory) = nil error, want error")
	}

	if _, _, err := LocalFilePath("file:///definitely/missing/file.html"); err == nil {
		t.Error("LocalFilePath(missing file URL) = nil error, want error")
	}
}

func TestFetchLocalHTML(t *testing.T) {
	path := writeTempFile(t, "page.html", `<html><head><title>Local</title></head><body><article><h1>Hello</h1><p>Local readable body text.</p></article></body></html>`)

	doc, err := FetchLocalFile(path, Options{})
	if err != nil {
		t.Fatalf("FetchLocalFile: %v", err)
	}
	if !doc.Converted || doc.Title != "Local" {
		t.Errorf("doc = %+v, want converted HTML with title", doc)
	}
	if !strings.Contains(doc.Content, "Hello") || !strings.Contains(doc.Content, "Local readable") {
		t.Errorf("content = %q, want extracted article", doc.Content)
	}
	if !strings.HasPrefix(doc.URL, "file://") {
		t.Errorf("URL = %q, want a file URL", doc.URL)
	}
}

func TestFetchLocalText(t *testing.T) {
	path := writeTempFile(t, "notes.txt", "plain text\n")
	doc, err := FetchLocalFile(path, Options{})
	if err != nil {
		t.Fatalf("FetchLocalFile: %v", err)
	}
	if doc.Converted || doc.Content != "plain text\n" {
		t.Errorf("doc = %+v, want verbatim text", doc)
	}
}

func TestFetchLocalBinaryRejected(t *testing.T) {
	path := writeTempFile(t, "file.pdf", "%PDF-1.7")
	if _, err := FetchLocalFile(path, Options{}); err == nil || !strings.Contains(err.Error(), "unsupported content type") {
		t.Fatalf("error = %v, want unsupported content type", err)
	}
}

func TestFetchLocalTruncates(t *testing.T) {
	path := writeTempFile(t, "notes.txt", strings.Repeat("a", 100))
	doc, err := FetchLocalFile(path, Options{MaxBytes: 10})
	if err != nil {
		t.Fatalf("FetchLocalFile: %v", err)
	}
	if !doc.Truncated || doc.Bytes != 10 || len(doc.Content) != 10 {
		t.Errorf("doc = truncated:%v bytes:%d len:%d, want 10", doc.Truncated, doc.Bytes, len(doc.Content))
	}
}
