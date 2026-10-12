package chat

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestExecuteWebSearch_invalidJSON(t *testing.T) {
	got := executeWebSearch("{bad json}")
	if !strings.Contains(got, "invalid arguments") {
		t.Errorf("executeWebSearch(bad json) = %q, want 'invalid arguments'", got)
	}
}

func TestExecuteWebSearch_missingQuery(t *testing.T) {
	for _, args := range []string{`{}`, `{"query":""}`, `{"query":"   "}`} {
		got := executeWebSearch(args)
		if !strings.Contains(got, "query is required") {
			t.Errorf("executeWebSearch(%s) = %q, want 'query is required'", args, got)
		}
	}
}

func TestFormatSearchResults(t *testing.T) {
	t.Run("renders provider, titles and urls", func(t *testing.T) {
		result := &search.SearchResult{
			Provider: "duckduckgo",
			Results: []search.Result{
				{URL: "https://a.example/1", Title: "Alpha"},
				{URL: "https://b.example/2", Title: "Beta"},
			},
		}
		got := formatSearchResults("golang", result)
		for _, want := range []string{
			"golang", "duckduckgo",
			"Alpha", "https://a.example/1",
			"Beta", "https://b.example/2",
			"web_fetch",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("formatSearchResults() = %q, missing %q", got, want)
			}
		}
	})

	t.Run("url-only result is not printed twice", func(t *testing.T) {
		result := &search.SearchResult{
			Provider: "duckduckgo",
			Results:  []search.Result{{URL: "https://only.example/1", Title: "https://only.example/1"}},
		}
		got := formatSearchResults("q", result)
		if n := strings.Count(got, "https://only.example/1"); n != 1 {
			t.Errorf("formatSearchResults() = %q, URL appears %d times, want 1", got, n)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		got := formatSearchResults("nothing", &search.SearchResult{Provider: "duckduckgo"})
		if !strings.Contains(got, "No search results") {
			t.Errorf("formatSearchResults(empty) = %q, want 'No search results'", got)
		}
	})

	t.Run("nil result", func(t *testing.T) {
		got := formatSearchResults("nothing", nil)
		if !strings.Contains(got, "No search results") {
			t.Errorf("formatSearchResults(nil) = %q, want 'No search results'", got)
		}
	})
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
	prev := options.Shared.Cfg
	options.Shared.Cfg = nil
	prevProvider := chatSearchProvider
	chatSearchProvider = ""
	t.Cleanup(func() { options.Shared.Cfg = prev; chatSearchProvider = prevProvider })

	stub := &stubProvider{results: []search.Result{{URL: "https://x.example", Title: "X"}}}
	router := search.NewRouter(search.NoopQuotaStore{})
	router.Register("stub", stub, &search.ProviderInfo{Tags: []string{"free"}})

	got, err := searchWithRouter(router, "hello", 3)
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

func TestSearchWithRouter_prefersNamedProvider(t *testing.T) {
	prev := options.Shared.Cfg
	options.Shared.Cfg = &types.Config{
		Defaults: &types.ConfigDefaults{
			Chat: &types.ChatDefaults{SearchProvider: "second"},
		},
	}
	prevProvider := chatSearchProvider
	chatSearchProvider = ""
	t.Cleanup(func() { options.Shared.Cfg = prev; chatSearchProvider = prevProvider })

	first := &stubProvider{results: []search.Result{{URL: "https://first", Title: "first"}}}
	second := &stubProvider{results: []search.Result{{URL: "https://second", Title: "second"}}}
	router := search.NewRouter(search.NoopQuotaStore{})
	router.Register("first", first, nil)
	router.Register("second", second, nil)

	got, err := searchWithRouter(router, "q", 1)
	if err != nil {
		t.Fatalf("searchWithRouter() error = %v", err)
	}
	if got.Provider != "second" {
		t.Errorf("provider = %q, want second", got.Provider)
	}
}

func TestSearchWithRouter_flagOverridesConfig(t *testing.T) {
	// Config says "second"; the --search-provider flag says "first" and must win.
	prev := options.Shared.Cfg
	options.Shared.Cfg = &types.Config{
		Defaults: &types.ConfigDefaults{
			Chat: &types.ChatDefaults{SearchProvider: "second"},
		},
	}
	prevProvider := chatSearchProvider
	chatSearchProvider = "first"
	t.Cleanup(func() { options.Shared.Cfg = prev; chatSearchProvider = prevProvider })

	first := &stubProvider{results: []search.Result{{URL: "https://first", Title: "first"}}}
	second := &stubProvider{results: []search.Result{{URL: "https://second", Title: "second"}}}
	router := search.NewRouter(search.NoopQuotaStore{})
	router.Register("first", first, nil)
	router.Register("second", second, nil)

	got, err := searchWithRouter(router, "q", 1)
	if err != nil {
		t.Fatalf("searchWithRouter() error = %v", err)
	}
	if got.Provider != "first" {
		t.Errorf("provider = %q, want first (flag overrides config)", got.Provider)
	}
}

func TestBuildAgentTools_webSearchPrivileged(t *testing.T) {
	prev := options.Shared.Cfg
	options.Shared.Cfg = nil
	t.Cleanup(func() { options.Shared.Cfg = prev })

	if toolNames(buildAgentTools(&types.ChatDefaults{}))["web_search"] {
		t.Error("web_search should be off by default")
	}

	options.Shared.Cfg = &types.Config{ToolsEnable: []string{"web_search"}}
	if !toolNames(buildAgentTools(&types.ChatDefaults{}))["web_search"] {
		t.Error("web_search should be enabled when listed in tools_enable")
	}
}
