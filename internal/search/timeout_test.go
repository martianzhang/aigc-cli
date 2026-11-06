package search

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestTimeoutFallbackAndOverride(t *testing.T) {
	prev := RequestTimeout()
	t.Cleanup(func() { SetDefaultTimeout(prev) })

	SetDefaultTimeout(0)
	if got := RequestTimeout(); got != DefaultTimeout {
		t.Errorf("RequestTimeout() = %v, want built-in %v", got, DefaultTimeout)
	}

	SetDefaultTimeout(5 * time.Second)
	if got := RequestTimeout(); got != 5*time.Second {
		t.Errorf("RequestTimeout() = %v, want 5s", got)
	}
}

// TestHTTPClientHonorsTimeout guards the provider client used by every search
// and fetch adapter.
func TestHTTPClientHonorsTimeout(t *testing.T) {
	prev := RequestTimeout()
	t.Cleanup(func() { SetDefaultTimeout(prev) })
	SetDefaultTimeout(80 * time.Millisecond)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	start := time.Now()
	resp, err := httpClient().Get(srv.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("httpClient().Get() error = nil, want a timeout")
	}
	if !strings.Contains(err.Error(), "Client.Timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("error = %v, want a client timeout", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("request took %v, want it to honor the 80ms timeout", elapsed)
	}
}
