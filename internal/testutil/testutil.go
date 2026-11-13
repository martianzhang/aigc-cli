// Package testutil provides small helpers shared by tests: an isolated home
// directory, deterministic HTTP rewrites, a network tripwire, golden files and
// polling.
package testutil

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TempHome points HOME (and USERPROFILE on Windows) at a fresh temp directory
// and returns it. Config/model code that calls os.UserHomeDir therefore never
// touches the developer's real profile.
func TempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	return home
}

// RewriteTransport routes every request to Target's host while preserving the
// path and query, so a client can be exercised against an httptest server
// without rewriting URLs in the test.
type RewriteTransport struct {
	Target string
	Base   http.RoundTripper
}

func (rt RewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(rt.Target)
	if err != nil {
		return nil, fmt.Errorf("parse rewrite target: %w", err)
	}
	clone := req.Clone(req.Context())
	clone.URL.Scheme = target.Scheme
	clone.URL.Host = target.Host
	clone.Host = target.Host
	base := rt.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}

// FailOnNetwork replaces http.DefaultClient with one whose transport fails the
// test on any request, and returns a restore function. Use it in dry-run /
// no-network contract tests.
func FailOnNetwork(t *testing.T) func() {
	t.Helper()
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: failingTransport{t: t}}
	return func() { http.DefaultClient = old }
}

type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if f.t != nil {
		f.t.Errorf("unexpected network request: %s %s", req.Method, req.URL)
	}
	return nil, errors.New("network disabled in this test")
}

// NewServer starts an httptest server running handler and closes it on cleanup.
func NewServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// ServerClient returns the test server's client, so requests trust its TLS
// certificate when the server is HTTPS.
func ServerClient(srv *httptest.Server) *http.Client {
	return srv.Client()
}

// Golden compares got with testdata/name. Set UPDATE_GOLDEN=1 to rewrite the
// file instead (review the diff before committing).
func Golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (set UPDATE_GOLDEN=1 to create it): %v", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("golden %s mismatch\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// Eventually polls cond until it returns true or timeout elapses. It is
// intended for goroutine/eventual-state assertions instead of fixed sleeps.
func Eventually(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}
