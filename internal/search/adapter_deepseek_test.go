package search

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewDeepSeekProviderDefaults(t *testing.T) {
	p := NewDeepSeekProvider("ds-key")
	if p.Name() != "deepseek" {
		t.Errorf("Name() = %q, want deepseek", p.Name())
	}
	if p.baseURL != defaultDeepSeekBaseURL {
		t.Errorf("baseURL = %q, want %q", p.baseURL, defaultDeepSeekBaseURL)
	}
	if p.baseURL != "https://api.deepseek.com" {
		t.Errorf("default base URL = %q, want https://api.deepseek.com", p.baseURL)
	}
}

// TestDeepSeekProviderImplementsCapabilities pins the two interfaces the chat
// primitives depend on: web_search needs a Provider, web_fetch needs a Fetcher.
func TestDeepSeekProviderImplementsCapabilities(t *testing.T) {
	p := NewDeepSeekProvider("ds-key")
	if _, ok := any(p).(Provider); !ok {
		t.Error("DeepSeekProvider must implement Provider")
	}
	if _, ok := any(p).(Fetcher); !ok {
		t.Error("DeepSeekProvider must implement Fetcher")
	}
}

func TestDeepSeekSearch(t *testing.T) {
	type captured struct {
		method, path, auth, contentType, query string
		count                                  int
	}
	var got captured

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		got.contentType = r.Header.Get("Content-Type")

		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Query string `json:"query"`
			Count int    `json:"count"`
		}
		_ = json.Unmarshal(body, &payload)
		got.query = payload.Query
		got.count = payload.Count

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[
			{"rerank_score":2.4,"title":"DeepSeek V4 发布","url":"https://example.com/a","snippet":"s","summary":"m","content":"c","site_name":"X","site_icon":null,"date":null,"authority":3},
			{"title":"Second","url":"https://example.com/b"}
		]}`)
	}))
	defer srv.Close()

	p := &DeepSeekProvider{apiKey: "ds-key", baseURL: srv.URL}
	results, err := p.Search("deepseek v4", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.path != "/v1/tools/search" {
		t.Errorf("path = %q, want /v1/tools/search", got.path)
	}
	if got.auth != "Bearer ds-key" {
		t.Errorf("Authorization = %q, want Bearer ds-key", got.auth)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}
	if got.query != "deepseek v4" {
		t.Errorf("query = %q, want deepseek v4", got.query)
	}
	if got.count != 5 {
		t.Errorf("count = %d, want 5", got.count)
	}

	want := []Result{
		{URL: "https://example.com/a", Title: "DeepSeek V4 发布"},
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

func TestDeepSeekFetch(t *testing.T) {
	var gotPath, gotAuth, gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(body, &payload)
		gotURL = payload.URL

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"title":"Example article","content":"# Example article\n\nBody."}`)
	}))
	defer srv.Close()

	p := &DeepSeekProvider{apiKey: "ds-key", baseURL: srv.URL}
	title, content, err := p.Fetch("https://example.com/article")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if gotPath != "/v1/tools/fetch" {
		t.Errorf("path = %q, want /v1/tools/fetch", gotPath)
	}
	if gotAuth != "Bearer ds-key" {
		t.Errorf("Authorization = %q, want Bearer ds-key", gotAuth)
	}
	if gotURL != "https://example.com/article" {
		t.Errorf("url = %q, want https://example.com/article", gotURL)
	}
	if title != "Example article" {
		t.Errorf("title = %q, want Example article", title)
	}
	if content != "# Example article\n\nBody." {
		t.Errorf("content = %q, want the markdown body", content)
	}
}

// An empty results array is a normal response (no matches) and must not be an
// error — DeepSeek bills per successful request with results, so no results is
// also the free case.
func TestDeepSeekSearchEmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[]}`)
	}))
	defer srv.Close()

	p := &DeepSeekProvider{apiKey: "ds-key", baseURL: srv.URL}
	results, err := p.Search("no matches", 5)
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

func TestDeepSeekSearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"invalid api key","code":"invalid_request_error"}}`)
	}))
	defer srv.Close()

	p := &DeepSeekProvider{apiKey: "bad", baseURL: srv.URL}
	if _, err := p.Search("q", 5); err == nil {
		t.Fatal("Search() error = nil, want error")
	}
	if _, _, err := p.Fetch("https://example.com/"); err == nil {
		t.Fatal("Fetch() error = nil, want error")
	}
}

func TestDeepSeekSearchBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `not json`)
	}))
	defer srv.Close()

	p := &DeepSeekProvider{apiKey: "ds-key", baseURL: srv.URL}
	if _, err := p.Search("q", 5); err == nil {
		t.Fatal("Search() error = nil, want error")
	}
}
