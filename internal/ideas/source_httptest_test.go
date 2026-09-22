package ideas

import (
	"net/http"
	"net/http/httptest"
	"net/url"
)

// overrideBaseURL points a package-level base URL var at a test server.
func overrideBaseURL(target *string, value string) func() {
	old := *target
	*target = value
	return func() { *target = old }
}

// newFixtureServer serves body and records the request path and query.
func newFixtureServer(body string, path *string, query *url.Values) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path != nil {
			*path = r.URL.Path
		}
		if query != nil {
			*query = r.URL.Query()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}
