package search

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewTencentWSAProviderDefaults(t *testing.T) {
	p := NewTencentWSAProvider("wsa-key")
	if p.Name() != "wsa" {
		t.Errorf("Name() = %q, want wsa", p.Name())
	}
	if p.baseURL != defaultTencentWSAURL {
		t.Errorf("baseURL = %q, want %q", p.baseURL, defaultTencentWSAURL)
	}
	if p.baseURL != "https://api.wsa.cloud.tencent.com" {
		t.Errorf("default base URL = %q, want https://api.wsa.cloud.tencent.com", p.baseURL)
	}
}

// TestTencentWSAProviderImplementsCapabilities pins the interface the router
// depends on. WSA's API KEY mode only exposes search, not page fetch.
func TestTencentWSAProviderImplementsCapabilities(t *testing.T) {
	p := NewTencentWSAProvider("wsa-key")
	if _, ok := any(p).(Provider); !ok {
		t.Error("TencentWSAProvider must implement Provider")
	}
	if _, ok := any(p).(Fetcher); ok {
		t.Error("TencentWSAProvider must not implement Fetcher")
	}
}

func TestTencentWSASearch(t *testing.T) {
	type captured struct {
		method, path, auth, contentType, query string
		cnt                                    int
	}
	var got captured

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		got.contentType = r.Header.Get("Content-Type")

		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Query string `json:"Query"`
			Cnt   int    `json:"Cnt"`
		}
		_ = json.Unmarshal(body, &payload)
		got.query = payload.Query
		got.cnt = payload.Cnt

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"Response":{"Pages":[
			"{\"passage\":\"a\",\"score\":0.9,\"title\":\"三星堆的由来\",\"url\":\"https://example.com/a\",\"site\":\"网易\"}",
			"{\"passage\":\"b\",\"title\":\"Second\",\"url\":\"https://example.com/b\"}"
		],"Query":"三星堆的由来","RequestId":"req_1"}}`)
	}))
	defer srv.Close()

	p := &TencentWSAProvider{apiKey: "wsa-key", baseURL: srv.URL}
	results, err := p.Search("三星堆的由来", 3)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.path != "/SearchPro" {
		t.Errorf("path = %q, want /SearchPro", got.path)
	}
	if got.auth != "Bearer wsa-key" {
		t.Errorf("Authorization = %q, want Bearer wsa-key", got.auth)
	}
	if got.contentType != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got.contentType)
	}
	if got.query != "三星堆的由来" {
		t.Errorf("Query = %q, want 三星堆的由来", got.query)
	}
	if got.cnt != 10 {
		t.Errorf("Cnt = %d, want 10 (smallest accepted bucket)", got.cnt)
	}

	want := []Result{
		{URL: "https://example.com/a", Title: "三星堆的由来"},
		{URL: "https://example.com/b", Title: "Second"},
	}
	if len(results) != len(want) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(want))
	}
	for i := range want {
		if results[i] != want[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], want[i])
		}
	}
}

// Cnt only accepts buckets of 10+, so a limit below the bucket must still trim.
func TestTencentWSASearchTrimsToLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"Response":{"Pages":[
			"{\"title\":\"a\",\"url\":\"https://example.com/a\"}",
			"{\"title\":\"b\",\"url\":\"https://example.com/b\"}",
			"{\"title\":\"c\",\"url\":\"https://example.com/c\"}"
		]}}`)
	}))
	defer srv.Close()

	p := &TencentWSAProvider{apiKey: "wsa-key", baseURL: srv.URL}
	results, err := p.Search("q", 2)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Title != "a" || results[1].Title != "b" {
		t.Errorf("results = %+v, want the first two pages", results)
	}
}

func TestTencentWSASearchEmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"Response":{"Pages":[],"Query":"q","RequestId":"req_1"}}`)
	}))
	defer srv.Close()

	p := &TencentWSAProvider{apiKey: "wsa-key", baseURL: srv.URL}
	results, err := p.Search("q", 5)
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

// Tencent's error envelope carries Error.Code/Message; it must reach the caller
// so an invalid key or unopened service is diagnosable.
func TestTencentWSASearchAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"Response":{"Error":{"Code":"UnauthorizedOperation","Message":"未授权操作"},"RequestId":"req_1"}}`)
	}))
	defer srv.Close()

	p := &TencentWSAProvider{apiKey: "bad", baseURL: srv.URL}
	_, err := p.Search("q", 5)
	if err == nil {
		t.Fatal("Search() error = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "UnauthorizedOperation") || !strings.Contains(got, "未授权操作") {
		t.Errorf("error = %q, want it to include code and message", got)
	}
}

func TestTencentWSASearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, `bad gateway`)
	}))
	defer srv.Close()

	p := &TencentWSAProvider{apiKey: "wsa-key", baseURL: srv.URL}
	if _, err := p.Search("q", 5); err == nil {
		t.Fatal("Search() error = nil, want error")
	}
}

func TestTencentWSASearchBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `not json`)
	}))
	defer srv.Close()

	p := &TencentWSAProvider{apiKey: "wsa-key", baseURL: srv.URL}
	if _, err := p.Search("q", 5); err == nil {
		t.Fatal("Search() error = nil, want error")
	}
}

func TestTencentWSASearchBadPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"Response":{"Pages":["not json"]}}`)
	}))
	defer srv.Close()

	p := &TencentWSAProvider{apiKey: "wsa-key", baseURL: srv.URL}
	if _, err := p.Search("q", 5); err == nil {
		t.Fatal("Search() error = nil, want page parse error")
	}
}

func TestTencentWSACntBuckets(t *testing.T) {
	tests := []struct {
		limit int
		want  int
	}{
		{limit: 0, want: 10},
		{limit: 3, want: 10},
		{limit: 10, want: 10},
		{limit: 11, want: 20},
		{limit: 20, want: 20},
		{limit: 21, want: 30},
		{limit: 50, want: 50},
		{limit: 99, want: 50},
	}
	for _, tc := range tests {
		if got := tencentResultCount(tc.limit); got != tc.want {
			t.Errorf("tencentResultCount(%d) = %d, want %d", tc.limit, got, tc.want)
		}
	}
}

func TestTencentWSAPreview(t *testing.T) {
	p := NewTencentWSAProvider("sk-secret-key")
	plan, err := p.Preview("三星堆的由来", 5, Options{})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if plan.Method != http.MethodPost {
		t.Errorf("method = %q, want POST", plan.Method)
	}
	if plan.URL != "https://api.wsa.cloud.tencent.com/SearchPro" {
		t.Errorf("url = %q, want SearchPro endpoint", plan.URL)
	}
	if len(plan.Headers) != 1 || plan.Headers[0].Name != "Authorization" {
		t.Fatalf("headers = %+v, want one Authorization header", plan.Headers)
	}
	if strings.Contains(plan.Headers[0].Value, "secret") {
		t.Errorf("Authorization = %q, want masked key", plan.Headers[0].Value)
	}
	if !strings.Contains(fmt.Sprintf("%v", plan.Body), "三星堆的由来") {
		t.Errorf("body = %v, want it to contain the query", plan.Body)
	}
}
