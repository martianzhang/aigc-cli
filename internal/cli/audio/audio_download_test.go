package audio

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/audio"
)

// cleanTarBz2Base64 is a tar.bz2 containing top/, top/sub/,
// top/model.onnx ("weights") and top/sub/tokens.txt ("hello").
const cleanTarBz2Base64 = "QlpoOTFBWSZTWbCgsecAAMZfkMmAQAH/hAABEER2797ABAABCDAAuIJVAaAAAAANMhgBk00GQwQ0xGjAVSFTzU9RGjGgGkGmnpDVO3eymlK8/8u04WCNl6WRU2LJjSupMvKkZpETESPvR0yJXGthOERU3zERx5Y43mJIM1edClESWwSaGtywVtZjS6iI5gkx+4799UlL7Jr022pxUqWQQCgaROEB92DVUoumNAsHYYnwY/F3JFOFCQsKCx5w"

// evilTarBz2Base64 is the same archive plus a top/../../escaped.txt entry.
const evilTarBz2Base64 = "QlpoOTFBWSZTWQukOOIAAJzfkMmAQAH3hAAIEARuh97ABAAAKCAAkgyhNNGgAaADTIEUppNohoaBtQ0NATfPu6RfY/qEJaAlZhm2OBXUN8pVNzNC2FKhCHSbTLXmSIscCESbAiY40raMA88ZqKCFC4Z6ChbsVpnnC1pBG+pe/oIfi7SMqYsYiG0KTIYNQWxMLDd8yj432VMJH4u5IpwoSAXSHHEA"

type testTarEntry struct {
	name string
	body string
	typ  byte
	link string
}

func newTestTarReader(t *testing.T, entries []testTarEntry) *tar.Reader {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0644, Typeflag: tar.TypeReg}
		if e.typ != 0 {
			hdr.Typeflag = e.typ
		}
		if hdr.Typeflag == tar.TypeDir {
			hdr.Mode = 0755
		}
		if e.link != "" {
			hdr.Linkname = e.link
		}
		if hdr.Typeflag == tar.TypeReg {
			hdr.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %q: %v", e.name, err)
		}
		if hdr.Typeflag == tar.TypeReg && e.body != "" {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("write body %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	return tar.NewReader(&buf)
}

func writeTarBz2Fixture(t *testing.T, encoded string) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "fixture.tar.bz2")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestExtractTar_RejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	extractDir := filepath.Join(root, "outer", "inner", "model")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("mkdir extract dir: %v", err)
	}

	tarr := newTestTarReader(t, []testTarEntry{
		{name: "top/", typ: tar.TypeDir},
		{name: "top/model.onnx", body: "good"},
		{name: "top/../../escaped.txt", body: "pwned"},
	})

	if err := extractTar(tarr, extractDir); err == nil {
		t.Fatal("expected extraction to fail on the traversal entry")
	}
	escaped := filepath.Join(root, "outer", "escaped.txt")
	if _, err := os.Stat(escaped); !os.IsNotExist(err) {
		t.Errorf("traversal entry escaped the extraction dir: %s (stat err %v)", escaped, err)
	}
}

func TestExtractTar_ExtractsNormalArchive(t *testing.T) {
	extractDir := filepath.Join(t.TempDir(), "model")
	tarr := newTestTarReader(t, []testTarEntry{
		{name: "top/", typ: tar.TypeDir},
		{name: "top/sub/", typ: tar.TypeDir},
		{name: "top/model.onnx", body: "weights"},
		{name: "top/sub/tokens.txt", body: "hello"},
	})

	if err := extractTar(tarr, extractDir); err != nil {
		t.Fatalf("extractTar(normal archive) = %v, want nil", err)
	}
	for rel, want := range map[string]string{"model.onnx": "weights", "sub/tokens.txt": "hello"} {
		got, err := os.ReadFile(filepath.Join(extractDir, rel))
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

func TestExtractTar_SkipsLinkEntries(t *testing.T) {
	extractDir := filepath.Join(t.TempDir(), "model")
	tarr := newTestTarReader(t, []testTarEntry{
		{name: "top/", typ: tar.TypeDir},
		{name: "top/escape", typ: tar.TypeSymlink, link: "../../../etc/passwd"},
		{name: "top/hard", typ: tar.TypeLink, link: "/etc/passwd"},
		{name: "top/ok.txt", body: "ok"},
	})

	if err := extractTar(tarr, extractDir); err != nil {
		t.Fatalf("extractTar(archive with links) = %v, want nil", err)
	}
	for _, name := range []string{"escape", "hard"} {
		if _, err := os.Lstat(filepath.Join(extractDir, name)); !os.IsNotExist(err) {
			t.Errorf("link entry %q should have been skipped (lstat err %v)", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(extractDir, "ok.txt")); err != nil {
		t.Errorf("regular file after link entries was not extracted: %v", err)
	}
}

func TestExtractTarBz2_RejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	extractDir := filepath.Join(root, "outer", "inner", "model")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		t.Fatalf("mkdir extract dir: %v", err)
	}

	archive := writeTarBz2Fixture(t, evilTarBz2Base64)
	if err := extractTarBz2(archive, extractDir); err == nil {
		t.Fatal("expected extraction of a traversal archive to fail")
	}
	escaped := filepath.Join(root, "outer", "escaped.txt")
	if _, err := os.Stat(escaped); !os.IsNotExist(err) {
		t.Errorf("traversal entry escaped the extraction dir: %s (stat err %v)", escaped, err)
	}
}

func TestExtractTarBz2_ExtractsNormalArchive(t *testing.T) {
	extractDir := filepath.Join(t.TempDir(), "model")
	archive := writeTarBz2Fixture(t, cleanTarBz2Base64)

	if err := extractTarBz2(archive, extractDir); err != nil {
		t.Fatalf("extractTarBz2(normal archive) = %v, want nil", err)
	}
	for rel, want := range map[string]string{"model.onnx": "weights", "sub/tokens.txt": "hello"} {
		got, err := os.ReadFile(filepath.Join(extractDir, rel))
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

func TestSanitizeModelName(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"kokoro", "kokoro", false},
		{"a/b/kokoro", "kokoro", false},
		{"../../../etc/passwd", "passwd", false},
		{"", "", true},
		{".", "", true},
		{"..", "", true},
		{"/", "", true},
	}
	for _, tc := range tests {
		got, err := sanitizeModelName(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("sanitizeModelName(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("sanitizeModelName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func testSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestVerifySHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.bin")
	body := []byte("hello model")
	if err := os.WriteFile(path, body, 0644); err != nil {
		t.Fatal(err)
	}

	if err := verifySHA256(path, testSHA256(body)); err != nil {
		t.Fatalf("verifySHA256(valid) error = %v", err)
	}
	if err := verifySHA256(path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("verifySHA256(mismatch) = nil, want error")
	}
	if err := verifySHA256(filepath.Join(t.TempDir(), "missing"), testSHA256(body)); err == nil {
		t.Fatal("verifySHA256(missing) = nil, want error")
	}
}

func TestDownloadSingleFileVerifiesHash(t *testing.T) {
	body := []byte("downloaded bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "model.onnx")

	if err := downloadSingleFile(srv.URL, dest, "test/model.onnx", false, testSHA256(body)); err != nil {
		t.Fatalf("downloadSingleFile(valid hash) error = %v", err)
	}
	if got, err := os.ReadFile(dest); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("downloaded file = %q, %v", got, err)
	}

	dest2 := filepath.Join(dir, "bad.onnx")
	err := downloadSingleFile(srv.URL, dest2, "test/bad.onnx", false, strings.Repeat("1", 64))
	if err == nil {
		t.Fatal("downloadSingleFile(bad hash) = nil, want error")
	}
	if _, statErr := os.Stat(dest2); !os.IsNotExist(statErr) {
		t.Errorf("hash mismatch must remove %s", dest2)
	}
}

func TestDownloadSingleFileSkipsVerifiedExisting(t *testing.T) {
	body := []byte("cached")
	dir := t.TempDir()
	dest := filepath.Join(dir, "cached.onnx")
	if err := os.WriteFile(dest, body, 0644); err != nil {
		t.Fatal(err)
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	if err := downloadSingleFile(srv.URL, dest, "test/cached.onnx", false, testSHA256(body)); err != nil {
		t.Fatalf("downloadSingleFile() error = %v", err)
	}
	if called {
		t.Error("verified existing file must not be re-downloaded")
	}
}

func audioRegistrySFXFiles(t *testing.T) []audio.ModelFile {
	t.Helper()
	info, err := audio.Lookup(audio.SFXDefaultModelID)
	if err != nil {
		t.Fatalf("lookup sfx model: %v", err)
	}
	return info.Files
}

func TestSFXModelFilesHaveHashes(t *testing.T) {
	files := audioRegistrySFXFiles(t)
	if len(files) < 8 {
		t.Fatalf("sfx file count = %d, want at least 8", len(files))
	}
	seen := map[string]bool{}
	for _, f := range files {
		if len(f.SHA256) != 64 {
			t.Errorf("%s: sha256 = %q, want 64 hex chars", f.Path, f.SHA256)
		}
		if _, err := hex.DecodeString(f.SHA256); err != nil {
			t.Errorf("%s: invalid sha256: %v", f.Path, err)
		}
		if seen[f.URL] {
			t.Errorf("duplicate URL %s", f.URL)
		}
		seen[f.URL] = true
	}
}
