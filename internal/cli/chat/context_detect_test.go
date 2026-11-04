package chat

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
)

func modelListServer(t *testing.T, data string, status int) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":%s}`, data)
	}))
	return srv, &calls
}

func TestProbeModelContextWindowSuccessAndCache(t *testing.T) {
	resetModelContextCache()
	t.Cleanup(resetModelContextCache)
	srv, calls := modelListServer(t, `[{"id":"custom-model","context_length":64000}]`, http.StatusOK)
	defer srv.Close()
	c := client.New("sk-test", srv.URL, "")

	if got := probeModelContextWindow(c, "custom-model", time.Second); got != 64000 {
		t.Fatalf("probe = %d, want 64000", got)
	}
	if got := probeModelContextWindow(c, "custom-model", time.Second); got != 64000 {
		t.Fatalf("cached probe = %d, want 64000", got)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("GET /models calls = %d, want 1 (cache)", got)
	}
}

func TestProbeModelContextWindowAliasesAndSuffix(t *testing.T) {
	resetModelContextCache()
	t.Cleanup(resetModelContextCache)
	cases := []string{
		`[{"id":"alias-model","context_window":32000}]`,
		`[{"id":"alias-model","max_context_length":48000}]`,
		`[{"id":"alias-model","top_provider":{"context_length":96000}}]`,
	}
	for _, data := range cases {
		srv, _ := modelListServer(t, data, http.StatusOK)
		c := client.New("sk-test", srv.URL, "")
		if got := probeModelContextWindow(c, "alias-model", time.Second); got == 0 {
			t.Fatalf("probe with %s returned 0", data)
		}
		srv.Close()
		resetModelContextCache()
	}

	// Requested without the provider prefix: a unique suffix match is enough.
	srv, _ := modelListServer(t, `[{"id":"openai/custom","context_length":128000}]`, http.StatusOK)
	defer srv.Close()
	c := client.New("sk-test", srv.URL, "")
	if got := probeModelContextWindow(c, "custom", time.Second); got != 128000 {
		t.Fatalf("suffix probe = %d, want 128000", got)
	}
}

func TestProbeModelContextWindowAmbiguousSuffixRejected(t *testing.T) {
	resetModelContextCache()
	t.Cleanup(resetModelContextCache)
	srv, _ := modelListServer(t, `[
		{"id":"a/same","context_length":32000},
		{"id":"b/same","context_length":64000}
	]`, http.StatusOK)
	defer srv.Close()
	c := client.New("sk-test", srv.URL, "")
	if got := probeModelContextWindow(c, "same", time.Second); got != 0 {
		t.Fatalf("ambiguous probe = %d, want 0", got)
	}
}

func TestProbeModelContextWindowNegativeCache(t *testing.T) {
	resetModelContextCache()
	t.Cleanup(resetModelContextCache)
	srv, calls := modelListServer(t, `[]`, http.StatusInternalServerError)
	defer srv.Close()
	c := client.New("sk-test", srv.URL, "")

	if got := probeModelContextWindow(c, "missing", time.Second); got != 0 {
		t.Fatalf("probe = %d, want 0", got)
	}
	if got := probeModelContextWindow(c, "missing", time.Second); got != 0 {
		t.Fatalf("cached probe = %d, want 0", got)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("GET /models calls = %d, want 1 (negative cache)", got)
	}
}

func TestProbeModelContextWindowTimeout(t *testing.T) {
	resetModelContextCache()
	t.Cleanup(resetModelContextCache)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	c := client.New("sk-test", srv.URL, "")

	start := time.Now()
	if got := probeModelContextWindow(c, "slow", 50*time.Millisecond); got != 0 {
		t.Fatalf("probe = %d, want 0", got)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("probe took %v, want a bounded timeout", elapsed)
	}
}

func TestModelContextCacheExpires(t *testing.T) {
	resetModelContextCache()
	t.Cleanup(resetModelContextCache)
	storeModelContextCache("key", 123, -time.Second)
	if _, ok := loadModelContextCache("key"); ok {
		t.Fatal("expired cache entry was returned")
	}
}

func TestProbeModelContextWindowCacheIsPerProvider(t *testing.T) {
	resetModelContextCache()
	t.Cleanup(resetModelContextCache)
	srvA, _ := modelListServer(t, `[{"id":"m","context_length":32000}]`, http.StatusOK)
	defer srvA.Close()
	srvB, _ := modelListServer(t, `[{"id":"m","context_length":64000}]`, http.StatusOK)
	defer srvB.Close()

	if got := probeModelContextWindow(client.New("k", srvA.URL, ""), "m", time.Second); got != 32000 {
		t.Fatalf("provider A = %d, want 32000", got)
	}
	if got := probeModelContextWindow(client.New("k", srvB.URL, ""), "m", time.Second); got != 64000 {
		t.Fatalf("provider B = %d, want 64000", got)
	}
}
