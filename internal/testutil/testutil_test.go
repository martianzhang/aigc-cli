package testutil

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTempHome(t *testing.T) {
	home := TempHome(t)
	if home == "" {
		t.Fatal("TempHome returned an empty path")
	}
	if got := os.Getenv("HOME"); got != home {
		t.Fatalf("HOME = %q, want %q", got, home)
	}
}

func TestRewriteTransportSendsToServer(t *testing.T) {
	srv := NewServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		_, _ = io.WriteString(w, "ok")
	})
	client := &http.Client{Transport: RewriteTransport{Target: srv.URL}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.example.com/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}

func TestFailingTransportRejectsRequests(t *testing.T) {
	client := &http.Client{Transport: failingTransport{}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("Do() error = nil, want network tripwire")
	}
}

func TestEventually(t *testing.T) {
	done := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(done)
	}()
	Eventually(t, time.Second, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
}

func TestGoldenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	t.Setenv("UPDATE_GOLDEN", "1")
	Golden(t, "sample.txt", []byte("hello"))
	t.Setenv("UPDATE_GOLDEN", "")
	Golden(t, "sample.txt", []byte("hello"))
}

func TestReadAllString(t *testing.T) {
	got, err := ReadAllString(strings.NewReader("hi"))
	if err != nil || got != "hi" {
		t.Fatalf("ReadAllString() = (%q, %v)", got, err)
	}
}
