package knowledge

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchTimeoutFallbackAndOverride(t *testing.T) {
	prev := currentFetchTimeout()
	t.Cleanup(func() { SetFetchTimeout(prev) })

	SetFetchTimeout(0)
	if got := currentFetchTimeout(); got != DefaultFetchTimeout {
		t.Errorf("currentFetchTimeout() = %v, want built-in %v", got, DefaultFetchTimeout)
	}

	SetFetchTimeout(3 * time.Second)
	if got := currentFetchTimeout(); got != 3*time.Second {
		t.Errorf("currentFetchTimeout() = %v, want 3s", got)
	}
}

// TestFetchURLHonorsConfiguredTimeout guards that KB document fetches follow
// the top-level timeout instead of the old hardcoded 30s.
func TestFetchURLHonorsConfiguredTimeout(t *testing.T) {
	prevTimeout := currentFetchTimeout()
	prevGuard := fetchValidateURL
	t.Cleanup(func() {
		SetFetchTimeout(prevTimeout)
		fetchValidateURL = prevGuard
	})

	SetFetchTimeout(80 * time.Millisecond)
	fetchValidateURL = func(string) error { return nil }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>late</body></html>"))
	}))
	defer srv.Close()

	start := time.Now()
	if _, err := FetchURL(srv.URL); err == nil {
		t.Fatal("FetchURL() error = nil, want a timeout")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("fetch took %v, want it to honor the 80ms timeout", elapsed)
	}
}
