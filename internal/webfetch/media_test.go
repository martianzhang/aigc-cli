package webfetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func mediaServer(t *testing.T, contentType string, body []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write(body)
	}))
}

func TestFetchSavesMedia(t *testing.T) {
	body := []byte("fake mp4 bytes")
	srv := mediaServer(t, "video/mp4", body)
	defer srv.Close()

	dir := t.TempDir()
	doc, err := Fetch(context.Background(), srv.URL+"/clip", Options{
		ValidateURL: allowAll,
		MediaDir:    dir,
		MediaName:   "rain-01",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !doc.Media {
		t.Fatal("Media = false, want true")
	}
	wantPath := filepath.Join(dir, "rain-01.mp4")
	if doc.SavedPath != wantPath {
		t.Errorf("SavedPath = %q, want %q", doc.SavedPath, wantPath)
	}
	got, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("saved body = %q, want %q", got, body)
	}
	if doc.Bytes != int64(len(body)) {
		t.Errorf("Bytes = %d, want %d", doc.Bytes, len(body))
	}
	sum := sha256.Sum256(body)
	if doc.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("SHA256 = %q, want %q", doc.SHA256, hex.EncodeToString(sum[:]))
	}
	if doc.MediaType != "video/mp4" {
		t.Errorf("MediaType = %q, want video/mp4", doc.MediaType)
	}
}

func TestFetchMediaExplicitNameWithExtension(t *testing.T) {
	srv := mediaServer(t, "image/jpeg", []byte("jpeg"))
	defer srv.Close()

	dir := t.TempDir()
	doc, err := Fetch(context.Background(), srv.URL+"/photo.jpg", Options{
		ValidateURL: allowAll,
		MediaDir:    dir,
		MediaName:   "still.custom",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if doc.SavedPath != filepath.Join(dir, "still.custom") {
		t.Errorf("SavedPath = %q, want the explicit name verbatim", doc.SavedPath)
	}
}

func TestFetchMediaNoNameUsesURL(t *testing.T) {
	srv := mediaServer(t, "audio/mpeg", []byte("mp3"))
	defer srv.Close()

	dir := t.TempDir()
	doc, err := Fetch(context.Background(), srv.URL+"/assets/theme.mp3", Options{
		ValidateURL: allowAll,
		MediaDir:    dir,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if doc.SavedPath != filepath.Join(dir, "theme.mp3") {
		t.Errorf("SavedPath = %q, want theme.mp3", doc.SavedPath)
	}
}

func TestFetchMediaContentTypeWinsOverURLSuffix(t *testing.T) {
	srv := mediaServer(t, "video/mp4", []byte("mp4"))
	defer srv.Close()

	dir := t.TempDir()
	doc, err := Fetch(context.Background(), srv.URL+"/download.php", Options{
		ValidateURL: allowAll,
		MediaDir:    dir,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if doc.SavedPath != filepath.Join(dir, "download.mp4") {
		t.Errorf("SavedPath = %q, want download.mp4", doc.SavedPath)
	}
}

func TestFetchMediaRefusesOverwriteUnlessForced(t *testing.T) {
	srv := mediaServer(t, "video/mp4", []byte("new"))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Fetch(context.Background(), srv.URL, Options{
		ValidateURL: allowAll,
		MediaDir:    dir,
		MediaName:   "clip",
	})
	if err == nil || !strings.Contains(err.Error(), "file exists") {
		t.Fatalf("error = %v, want file-exists refusal", err)
	}

	if _, err := Fetch(context.Background(), srv.URL, Options{
		ValidateURL: allowAll,
		MediaDir:    dir,
		MediaName:   "clip",
		Force:       true,
	}); err != nil {
		t.Fatalf("forced Fetch: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("overwritten body = %q, want new", got)
	}
}

func TestFetchMediaRejectsPathTraversal(t *testing.T) {
	srv := mediaServer(t, "video/mp4", []byte("x"))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, Options{
		ValidateURL: allowAll,
		MediaDir:    t.TempDir(),
		MediaName:   "../escape",
	})
	if err == nil || !strings.Contains(err.Error(), "path separator") {
		t.Fatalf("error = %v, want a path-separator rejection", err)
	}
}

func TestFetchMediaRespectsSizeLimit(t *testing.T) {
	srv := mediaServer(t, "video/mp4", []byte("0123456789"))
	defer srv.Close()

	dir := t.TempDir()
	_, err := Fetch(context.Background(), srv.URL, Options{
		ValidateURL:   allowAll,
		MediaDir:      dir,
		MediaName:     "clip",
		MediaMaxBytes: 4,
	})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("error = %v, want a size-limit error", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "clip.mp4")); !os.IsNotExist(statErr) {
		t.Errorf("partial file left behind: %v", statErr)
	}
}

func TestFetchTextWithMediaDirStillPrintsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	doc, err := Fetch(context.Background(), srv.URL, Options{
		ValidateURL: allowAll,
		MediaDir:    dir,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if doc.Media {
		t.Error("Media = true for a text response")
	}
	if doc.Content != "hello" {
		t.Errorf("Content = %q, want hello", doc.Content)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("output dir contains %d entries, want none", len(entries))
	}
}

func TestFetchMediaReportsProgress(t *testing.T) {
	body := []byte(strings.Repeat("x", 64*1024))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	var calls int
	var lastDone, lastTotal int64
	_, err := Fetch(context.Background(), srv.URL, Options{
		ValidateURL: allowAll,
		MediaDir:    t.TempDir(),
		MediaName:   "clip",
		Progress: func(done, total int64) {
			calls++
			lastDone, lastTotal = done, total
		},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if calls == 0 {
		t.Fatal("progress callback was never called")
	}
	if lastDone != int64(len(body)) {
		t.Errorf("last done = %d, want %d", lastDone, len(body))
	}
	if lastTotal != int64(len(body)) {
		t.Errorf("last total = %d, want Content-Length %d", lastTotal, len(body))
	}
}
