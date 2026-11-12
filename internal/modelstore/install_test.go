package modelstore

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

func sum(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

func TestInstallVerifiesChecksum(t *testing.T) {
	const body = "model-bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.onnx")
	if err := Install(context.Background(), srv.Client(), srv.URL, dest, Options{SHA256: sum(body)}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != body {
		t.Fatalf("installed file = %q, %v", got, err)
	}
}

func TestInstallRejectsChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tampered"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.onnx")
	if err := Install(context.Background(), srv.Client(), srv.URL, dest, Options{SHA256: sum("expected")}); err == nil {
		t.Fatal("Install() error = nil, want checksum mismatch")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("corrupt file must not be installed")
	}
}

func TestInstallResumesPartialDownload(t *testing.T) {
	const body = "0123456789"
	var ranges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))
		start := 0
		if raw := strings.TrimPrefix(r.Header.Get("Range"), "bytes="); raw != "" {
			if v, err := strconv.Atoi(strings.TrimSuffix(raw, "-")); err == nil {
				start = v
			}
		}
		w.Header().Set("Content-Range", "bytes "+strconv.Itoa(start)+"-9/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte(body[start:]))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.onnx")
	if err := os.WriteFile(dest+".part", []byte(body[:4]), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), srv.Client(), srv.URL, dest, Options{SHA256: sum(body)}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != body {
		t.Fatalf("installed file = %q, %v", got, err)
	}
	if len(ranges) != 1 || ranges[0] != "bytes=4-" {
		t.Fatalf("Range headers = %v, want [bytes=4-]", ranges)
	}
}

func TestInstallPinsTOFUChecksum(t *testing.T) {
	const body = "tofu-model"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.onnx")
	if err := Install(context.Background(), srv.Client(), srv.URL, dest, Options{}); err != nil {
		t.Fatalf("first Install() error = %v", err)
	}
	pin, err := os.ReadFile(dest + ".sha256")
	if err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}
	if strings.TrimSpace(string(pin)) != sum(body) {
		t.Fatalf("sidecar = %q, want %q", pin, sum(body))
	}
	if err := Install(context.Background(), srv.Client(), srv.URL, dest, Options{}); err != nil {
		t.Fatalf("second Install() error = %v", err)
	}
}

func TestInstallRejectsTamperedTOFUFile(t *testing.T) {
	const body = "trusted"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tampered"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "model.onnx")
	if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest+".sha256", []byte(sum(body)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), srv.Client(), srv.URL, dest, Options{Force: true}); err == nil {
		t.Fatal("Install() error = nil, want TOFU checksum mismatch")
	}
}

func TestInstallPinsLegacyFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "legacy.onnx")
	if err := os.WriteFile(dest, []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), nil, "http://unused.invalid", dest, Options{}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	pin, err := os.ReadFile(dest + ".sha256")
	if err != nil {
		t.Fatalf("legacy pin not written: %v", err)
	}
	if strings.TrimSpace(string(pin)) != sum("legacy") {
		t.Fatalf("pin = %q, want legacy digest", pin)
	}
}
