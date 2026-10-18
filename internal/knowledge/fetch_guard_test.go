package knowledge

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestFetchURL_RejectsLoopbackServer covers the integration point: the KB
// fetcher must run the netguard check before issuing a request. The guard's own
// unit tests live in internal/netguard.
func TestFetchURL_RejectsLoopbackServer(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("<html><body>secret</body></html>"))
	}))
	defer srv.Close()

	if _, err := FetchURL(srv.URL); err == nil {
		t.Fatal("FetchURL on a loopback httptest server should be rejected")
	}
	if hits.Load() != 0 {
		t.Errorf("guard rejected the URL but the server served %d request(s)", hits.Load())
	}
}
