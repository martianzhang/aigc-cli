package webtool

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/search"
)

func TestFormatSearchResults(t *testing.T) {
	t.Run("renders provider, titles and urls", func(t *testing.T) {
		result := &search.SearchResult{
			Provider: "duckduckgo",
			Results: []search.Result{
				{URL: "https://a.example/1", Title: "Alpha"},
				{URL: "https://b.example/2", Title: "Beta"},
			},
		}
		got := FormatSearchResults("golang", result, "web_fetch")
		for _, want := range []string{
			"golang", "duckduckgo",
			"Alpha", "https://a.example/1",
			"Beta", "https://b.example/2",
			"web_fetch",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("FormatSearchResults() = %q, missing %q", got, want)
			}
		}
	})

	t.Run("url-only result is not printed twice", func(t *testing.T) {
		result := &search.SearchResult{
			Provider: "duckduckgo",
			Results:  []search.Result{{URL: "https://only.example/1", Title: "https://only.example/1"}},
		}
		got := FormatSearchResults("q", result, "web_fetch")
		if n := strings.Count(got, "https://only.example/1"); n != 1 {
			t.Errorf("FormatSearchResults() = %q, URL appears %d times, want 1", got, n)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		got := FormatSearchResults("nothing", &search.SearchResult{Provider: "duckduckgo"}, "web_fetch")
		if !strings.Contains(got, "No search results") {
			t.Errorf("FormatSearchResults(empty) = %q, want 'No search results'", got)
		}
	})

	t.Run("nil result", func(t *testing.T) {
		got := FormatSearchResults("nothing", nil, "web_fetch")
		if !strings.Contains(got, "No search results") {
			t.Errorf("FormatSearchResults(nil) = %q, want 'No search results'", got)
		}
	})
}

func TestWebSearch_invalidArgs(t *testing.T) {
	if got := WebSearch("{bad json}", Env{}); !strings.Contains(got, "invalid arguments") {
		t.Errorf("WebSearch(bad json) = %q, want 'invalid arguments'", got)
	}
	for _, args := range []string{`{}`, `{"query":""}`, `{"query":"   "}`} {
		if got := WebSearch(args, Env{}); !strings.Contains(got, "query is required") {
			t.Errorf("WebSearch(%s) = %q, want 'query is required'", args, got)
		}
	}
}

// Without a configured third-party provider, web_search/web_fetch must fail with
// a message pointing at the right knob rather than falling back silently to the
// built-in engine.
func TestWebSearch_noRemoteProvider(t *testing.T) {
	got := WebSearch(`{"query":"hello"}`, Env{Scope: "defaults.chat.search_provider"})
	if !strings.Contains(got, "third-party provider") {
		t.Errorf("WebSearch(no provider) = %q, want a hint about configuring one", got)
	}
	if !strings.Contains(got, "defaults.chat.search_provider") {
		t.Errorf("WebSearch(no provider) = %q, want the scope hint", got)
	}
}

func TestWebFetch_noRemoteProvider(t *testing.T) {
	got := WebFetch(`{"url":"https://example.com/a"}`, Env{Scope: "search_provider"})
	if !strings.Contains(got, "no third-party fetch provider") {
		t.Errorf("WebFetch(no provider) = %q, want 'no third-party fetch provider'", got)
	}
	if !strings.Contains(got, "search_provider") {
		t.Errorf("WebFetch(no provider) = %q, want the scope hint", got)
	}
}

// stubProvider is an in-memory search.Provider for router tests.
type stubProvider struct {
	results []search.Result
	query   string
	limit   int
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) Search(query string, limit int) ([]search.Result, error) {
	s.query, s.limit = query, limit
	return s.results, nil
}

func TestSearchWithRouter_auto(t *testing.T) {
	stub := &stubProvider{results: []search.Result{{URL: "https://x.example", Title: "X"}}}
	router := search.NewRouter(search.NoopQuotaStore{})
	router.Register("stub", stub, &search.ProviderInfo{Tags: []string{"free"}})

	got, err := searchWithRouter(router, "hello", 3, "auto")
	if err != nil {
		t.Fatalf("searchWithRouter() error = %v", err)
	}
	if got.Provider != "stub" || len(got.Results) != 1 {
		t.Errorf("result = %+v, want provider stub with 1 result", got)
	}
	if stub.query != "hello" || stub.limit != 3 {
		t.Errorf("provider received (%q, %d), want (hello, 3)", stub.query, stub.limit)
	}
}

func TestSearchWithRouter_namedSpec(t *testing.T) {
	first := &stubProvider{results: []search.Result{{URL: "https://first", Title: "first"}}}
	second := &stubProvider{results: []search.Result{{URL: "https://second", Title: "second"}}}
	router := search.NewRouter(search.NoopQuotaStore{})
	router.Register("first", first, nil)
	router.Register("second", second, nil)

	got, err := searchWithRouter(router, "q", 1, "second")
	if err != nil {
		t.Fatalf("searchWithRouter() error = %v", err)
	}
	if got.Provider != "second" {
		t.Errorf("provider = %q, want second", got.Provider)
	}
}

// A named spec that is not registered must fail rather than silently picking
// another vendor.
func TestSearchWithRouter_unknownNamedSpecFails(t *testing.T) {
	first := &stubProvider{results: []search.Result{{URL: "https://first", Title: "first"}}}
	router := search.NewRouter(search.NoopQuotaStore{})
	router.Register("first", first, nil)

	if _, err := searchWithRouter(router, "q", 1, "missing"); err == nil {
		t.Error("searchWithRouter(unknown) = nil error, want a failure")
	}
}
