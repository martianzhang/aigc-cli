package search

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// stubFetcher implements both Provider and Fetcher.
type stubFetcher struct{ name string }

func (s *stubFetcher) Name() string                         { return s.name }
func (s *stubFetcher) Search(string, int) ([]Result, error) { return nil, nil }
func (s *stubFetcher) Fetch(url string) (string, string, error) {
	return "title", "content:" + url, nil
}

// searchOnly implements Provider but not Fetcher (e.g. DuckDuckGo, Brave).
type searchOnly struct{ name string }

func (s *searchOnly) Name() string                         { return s.name }
func (s *searchOnly) Search(string, int) ([]Result, error) { return nil, nil }

func TestResolveFetcher(t *testing.T) {
	router := NewRouter()
	router.Register("alpha", &searchOnly{name: "alpha"})
	router.Register("beta", &stubFetcher{name: "beta"})

	cases := []struct {
		name   string
		spec   string
		want   string
		wantOK bool
	}{
		{name: "named provider that fetches", spec: "beta", want: "beta", wantOK: true},
		{name: "named provider that cannot fetch", spec: "alpha", wantOK: false},
		{name: "unknown name", spec: "nope", wantOK: false},
		{name: "empty is not a provider", spec: "", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := ResolveFetcher(router, tc.spec)
			if ok != tc.wantOK {
				t.Fatalf("ResolveFetcher(%q) ok = %v, want %v", tc.spec, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if f.Name() != tc.want {
				t.Errorf("ResolveFetcher(%q) = %q, want %q", tc.spec, f.Name(), tc.want)
			}
			if title, content, err := f.Fetch("https://x.example/"); err != nil || title != "title" || content != "content:https://x.example/" {
				t.Errorf("Fetch() = (%q, %q, %v)", title, content, err)
			}
		})
	}
}

func TestFetcherNames(t *testing.T) {
	router := NewRouter()
	router.Register("alpha", &searchOnly{name: "alpha"})
	router.Register("beta", &stubFetcher{name: "beta"})
	router.Register("gamma", &stubFetcher{name: "gamma"})

	got := router.FetcherNames()
	if len(got) != 2 || got[0] != "beta" || got[1] != "gamma" {
		t.Errorf("FetcherNames() = %v, want [beta gamma]", got)
	}
}

func TestRegisterRemote(t *testing.T) {
	router := NewRouter()
	RegisterRemote(router, map[string]*types.WebSearchProvider{
		"ddg":      {Type: "duckduckgo"},
		"deepseek": {Type: "deepseek", APIKey: "k"},
		"brave":    {Type: "brave"}, // missing api_key -> skipped
	})

	if _, ok := router.GetProvider("ddg"); ok {
		t.Error("the built-in duckduckgo engine must be excluded from the remote router")
	}
	if _, ok := router.GetProvider("duckduckgo"); ok {
		t.Error("RegisterRemote must not add the built-in duckduckgo fallback")
	}
	if _, ok := router.GetProvider("deepseek"); !ok {
		t.Error("deepseek should be registered")
	}
	if _, ok := router.GetProvider("brave"); ok {
		t.Error("an unconstructable provider (missing api_key) should be skipped")
	}
}
