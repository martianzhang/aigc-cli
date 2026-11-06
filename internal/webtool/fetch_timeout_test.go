package webtool

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/search"
)

// TestFetchHonorsConfiguredTimeout guards that the chat/MCP built-in fetch
// follows the top-level timeout instead of its old hardcoded 30s.
func TestFetchHonorsConfiguredTimeout(t *testing.T) {
	allowLoopbackFetch(t)
	prev := search.RequestTimeout()
	search.SetDefaultTimeout(80 * time.Millisecond)
	t.Cleanup(func() { search.SetDefaultTimeout(prev) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>late</body></html>"))
	}))
	defer srv.Close()

	start := time.Now()
	got := Fetch(`{"url":"` + srv.URL + `"}`)
	if !strings.HasPrefix(got, "Error:") {
		t.Fatalf("Fetch = %q, want an error", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("fetch took %v, want it to honor the 80ms timeout", elapsed)
	}
}
