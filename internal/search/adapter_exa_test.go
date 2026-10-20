package search

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewExaProviderDefaults(t *testing.T) {
	p := NewExaProvider("exa-key")
	if p.Name() != "exa" {
		t.Errorf("Name() = %q, want exa", p.Name())
	}
	if p.baseURL != defaultExaBaseURL {
		t.Errorf("baseURL = %q, want %q", p.baseURL, defaultExaBaseURL)
	}
	if p.baseURL != "https://api.exa.ai" {
		t.Errorf("default base URL = %q, want https://api.exa.ai", p.baseURL)
	}
}

// TestExaProviderImplementsCapabilities pins the interfaces the chat primitives
// depend on: web_search needs a Provider, web_fetch needs a Fetcher.
func TestExaProviderImplementsCapabilities(t *testing.T) {
	p := NewExaProvider("exa-key")
	if _, ok := any(p).(Provider); !ok {
		t.Error("ExaProvider must implement Provider")
	}
	if _, ok := any(p).(Fetcher); !ok {
		t.Error("ExaProvider must implement Fetcher")
	}
}

func TestExaSearch(t *testing.T) {
	type captured struct {
		method, path, apiKey, contentType, query string
		numResults                               int
	}
	var got captured

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.apiKey = r.Header.Get("x-api-key")
		got.contentType = r.Header.Get("Content-Type")

		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Query      string `json:"query"`
			NumResults int    `json:"numResults"`
		}
		_ = json.Unmarshal(body, &payload)
		got.query = payload.Query
		got.numResults = payload.NumResults

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"requestId":"req_1","results":[
			{"id":"https://example.com/a","title":"Exa Search API","url":"https://example.com/a"},
			{"id":"https://example.com/b","title":"Second","url":"https://example.com/b"}
		]}`)
	}))
	defer srv.Close()

	p := &ExaProvider{apiKey: "exa-key", baseURL: srv.URL}
	results, err := p.Search("exa search api", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.path != "/search" {
		t.Errorf("path = %q, want /search", got.path)
	}
	if got.apiKey != "exa-key" {
		t.Errorf("x-api-key = %q, want exa-key", got.apiKey)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}
	if got.query != "exa search api" {
		t.Errorf("query = %q, want exa search api", got.query)
	}
	if got.numResults != 5 {
		t.Errorf("numResults = %d, want 5", got.numResults)
	}

	want := []Result{
		{URL: "https://example.com/a", Title: "Exa Search API"},
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

func TestExaFetch(t *testing.T) {
	var gotPath, gotAPIKey, gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			URLs []string `json:"urls"`
		}
		_ = json.Unmarshal(body, &payload)
		if len(payload.URLs) > 0 {
			gotURL = payload.URLs[0]
		}

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[{"title":"Example article","url":"https://example.com/article","text":"# Example article\n\nBody."}]}`)
	}))
	defer srv.Close()

	p := &ExaProvider{apiKey: "exa-key", baseURL: srv.URL}
	title, content, err := p.Fetch("https://example.com/article")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if gotPath != "/contents" {
		t.Errorf("path = %q, want /contents", gotPath)
	}
	if gotAPIKey != "exa-key" {
		t.Errorf("x-api-key = %q, want exa-key", gotAPIKey)
	}
	if gotURL != "https://example.com/article" {
		t.Errorf("url = %q, want https://example.com/article", gotURL)
	}
	if title != "Example article" {
		t.Errorf("title = %q, want Example article", title)
	}
	if content != "# Example article\n\nBody." {
		t.Errorf("content = %q, want the page text", content)
	}
}

func TestExaFetchNoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[]}`)
	}))
	defer srv.Close()

	p := &ExaProvider{apiKey: "exa-key", baseURL: srv.URL}
	if _, _, err := p.Fetch("https://example.com/"); err == nil {
		t.Fatal("Fetch() error = nil, want error when results is empty")
	}
}

// An empty results array is a normal response (no matches), not an error.
func TestExaSearchEmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"results":[]}`)
	}))
	defer srv.Close()

	p := &ExaProvider{apiKey: "exa-key", baseURL: srv.URL}
	results, err := p.Search("no matches", 5)
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

// Exa's error envelope carries a human-readable message; it must reach the
// caller so an invalid key or exhausted credit is diagnosable.
func TestExaSearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"requestId":"req_1","error":"Invalid API key","tag":"INVALID_API_KEY"}`)
	}))
	defer srv.Close()

	p := &ExaProvider{apiKey: "bad", baseURL: srv.URL}
	_, err := p.Search("q", 5)
	if err == nil {
		t.Fatal("Search() error = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "Invalid API key") {
		t.Errorf("error = %q, want it to include the API message", got)
	}
	if _, _, err := p.Fetch("https://example.com/"); err == nil {
		t.Fatal("Fetch() error = nil, want error")
	}
}

func TestExaSearchBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `not json`)
	}))
	defer srv.Close()

	p := &ExaProvider{apiKey: "exa-key", baseURL: srv.URL}
	if _, err := p.Search("q", 5); err == nil {
		t.Fatal("Search() error = nil, want error")
	}
}
