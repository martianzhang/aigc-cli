package audio

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var mirroredRuntimeAssets = []string{
	"libaigc-sherpa-helper.dylib",
	"libaigc-sherpa-helper.so",
	"aigc-sherpa-helper.dll",
	"libsherpa-onnx-c-api.dylib",
	"libsherpa-onnx-c-api.so",
	"sherpa-onnx-c-api.dll",
	"onnxruntime.dll",
}

func TestAudioRuntimeSHA256_valid(t *testing.T) {
	for name, sum := range audioRuntimeSHA256 {
		if len(sum) != 64 {
			t.Errorf("%s: digest %q is not 64 hex chars", name, sum)
		}
		if _, err := hex.DecodeString(sum); err != nil {
			t.Errorf("%s: digest %q is not hex: %v", name, sum, err)
		}
		if sum != strings.ToLower(sum) {
			t.Errorf("%s: digest %q must be lowercase", name, sum)
		}
		if name == "" || strings.ContainsAny(name, `/\`) {
			t.Errorf("%q is not a plain asset name", name)
		}
	}
}

func TestAudioRuntimeSHA256_coversMirroredAssets(t *testing.T) {
	for _, name := range mirroredRuntimeAssets {
		if _, ok := audioRuntimeSHA256[name]; !ok {
			t.Errorf("%s is downloaded from the mirror but has no pinned digest", name)
		}
	}
}

func TestDownloadAsset_refusesUnpinnedAsset(t *testing.T) {
	dir := t.TempDir()

	// libonnxruntime.so is not mirrored under the v1 release, so it must never
	// be fetched unverified.
	err := downloadAsset("http://127.0.0.1:1", "libonnxruntime.so", dir)
	if err == nil {
		t.Fatal("expected an error for an unpinned asset")
	}
	if !strings.Contains(err.Error(), "pinned SHA-256") {
		t.Fatalf("error = %q, want it to mention the missing pinned SHA-256", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "libonnxruntime.so")); statErr == nil {
		t.Fatal("unpinned asset must not be written")
	}
}

func TestDownloadAsset_skipsExistingFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "onnxruntime.dll")
	if err := os.WriteFile(dest, []byte("already installed"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A failing server proves the existing file short-circuits the download.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "must not be called", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := downloadAsset(srv.URL, "onnxruntime.dll", dir); err != nil {
		t.Fatalf("downloadAsset: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "already installed" {
		t.Fatalf("existing file content = %q, want it untouched", got)
	}
}

func TestDownloadVerified_rejectsTamperedPayload(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "libsherpa-onnx-c-api.dylib")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("tampered native library"))
	}))
	defer srv.Close()

	sum := sha256.Sum256([]byte("the real library"))
	err := downloadVerified(srv.URL+"/libsherpa-onnx-c-api.dylib", dest, hex.EncodeToString(sum[:]))
	if err == nil {
		t.Fatal("expected a checksum error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %q, want a checksum mismatch", err)
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		t.Fatal("tampered payload must not be installed")
	}
	if entries, readErr := os.ReadDir(dir); readErr != nil || len(entries) != 0 {
		t.Fatalf("dir entries = %v (err %v), want none (no .part leftovers)", entries, readErr)
	}
}

func TestDownloadVerified_installsMatchingPayload(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "libsherpa-onnx-c-api.dylib")
	payload := []byte("verified native library")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	sum := sha256.Sum256(payload)
	if err := downloadVerified(srv.URL+"/libsherpa-onnx-c-api.dylib", dest, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("downloadVerified: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("installed file missing: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("installed content = %q, want %q", got, payload)
	}
	if _, err := os.Stat(dest + ".sha256"); err == nil {
		t.Error(".sha256 sidecar should be removed once the digest is pinned in the binary")
	}
}
