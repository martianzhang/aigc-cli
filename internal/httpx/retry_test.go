package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoRetriesSafeRequest(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			http.Error(w, "busy", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := Do(context.Background(), srv.Client(), req, Policy{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
		MaxDelay:    5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

func TestDoDoesNotRetryPost(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "busy", http.StatusInternalServerError)
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)
	resp, err := Do(context.Background(), srv.Client(), req, Policy{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("POST attempts = %d, want 1", got)
	}
}

func TestDoStopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if _, err := Do(ctx, srv.Client(), req, Policy{MaxAttempts: 3, BaseDelay: time.Minute}); err == nil {
		t.Fatal("Do() error = nil, want context cancellation")
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("2"); got != 2*time.Second {
		t.Errorf("parseRetryAfter(2) = %v, want 2s", got)
	}
	if got := parseRetryAfter("not-a-date"); got != 0 {
		t.Errorf("parseRetryAfter(invalid) = %v, want 0", got)
	}
}
