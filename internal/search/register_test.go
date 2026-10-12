package search

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// Compile-time guarantee that NoopQuotaStore satisfies the production interface.
var _ QuotaStore = NoopQuotaStore{}

func TestNoopQuotaStore(t *testing.T) {
	var s NoopQuotaStore
	if !s.CanUse("anything") {
		t.Error("CanUse should always be true")
	}
	s.RecordUse("anything") // must not panic
	s.ResetIfNeeded("anything")
	if used, total := s.Usage("anything"); used != 0 || total != 0 {
		t.Errorf("Usage() = %d, %d, want 0, 0", used, total)
	}
}

func TestRegisterAll(t *testing.T) {
	tests := []struct {
		name      string
		providers map[string]*types.WebSearchProvider
		wantNames []string
	}{
		{
			name:      "nil config falls back to duckduckgo",
			providers: nil,
			wantNames: []string{"duckduckgo"},
		},
		{
			name:      "empty config falls back to duckduckgo",
			providers: map[string]*types.WebSearchProvider{},
			wantNames: []string{"duckduckgo"},
		},
		{
			name: "configured provider plus a fallback",
			providers: map[string]*types.WebSearchProvider{
				"firecrawl": {Type: "firecrawl", APIKey: "fc-key", Weight: 5},
			},
			wantNames: []string{"firecrawl", "duckduckgo"},
		},
		{
			name: "explicit duckduckgo is not duplicated",
			providers: map[string]*types.WebSearchProvider{
				"duckduckgo": {Type: "duckduckgo", Tags: []string{"free"}},
			},
			wantNames: []string{"duckduckgo"},
		},
		{
			name: "unconstructable provider is skipped",
			providers: map[string]*types.WebSearchProvider{
				"brave": {Type: "brave"}, // missing api_key
			},
			wantNames: []string{"duckduckgo"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRouter(NoopQuotaStore{})
			RegisterAll(r, tc.providers)

			for _, name := range tc.wantNames {
				if _, ok := r.GetProvider(name); !ok {
					t.Errorf("provider %q not registered", name)
				}
			}
			if len(r.providers) != len(tc.wantNames) {
				t.Errorf("registered %d providers, want %d", len(r.providers), len(tc.wantNames))
			}
		})
	}
}

func TestRegisterAllPreservesWeight(t *testing.T) {
	r := NewRouter(NoopQuotaStore{})
	RegisterAll(r, map[string]*types.WebSearchProvider{
		"firecrawl": {Type: "firecrawl", APIKey: "fc-key", Weight: 7},
	})
	if got := r.configs["firecrawl"]; got == nil || got.Weight != 7 {
		t.Errorf("firecrawl config = %+v, want weight 7", got)
	}
}

func TestResolveStrategy(t *testing.T) {
	tests := []struct {
		name          string
		provider      string
		wantStrategy  string
		wantPreferred []string
	}{
		{name: "empty is auto", provider: "", wantStrategy: "auto"},
		{name: "auto", provider: "auto", wantStrategy: "auto"},
		{name: "AUTO is case-insensitive", provider: "AUTO", wantStrategy: "auto"},
		{name: "free is cheap", provider: "free", wantStrategy: "cheap"},
		{name: "cheap", provider: "cheap", wantStrategy: "cheap"},
		{name: "quality", provider: "quality", wantStrategy: "quality"},
		{name: "named provider is manual", provider: "brave", wantStrategy: "manual", wantPreferred: []string{"brave"}},
		{name: "named provider trims whitespace", provider: "  doubao ", wantStrategy: "manual", wantPreferred: []string{"doubao"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStrategy, gotPreferred := ResolveStrategy(tc.provider)
			if gotStrategy != tc.wantStrategy {
				t.Errorf("strategy = %q, want %q", gotStrategy, tc.wantStrategy)
			}
			if len(gotPreferred) != len(tc.wantPreferred) {
				t.Fatalf("preferred = %v, want %v", gotPreferred, tc.wantPreferred)
			}
			for i := range gotPreferred {
				if gotPreferred[i] != tc.wantPreferred[i] {
					t.Errorf("preferred[%d] = %q, want %q", i, gotPreferred[i], tc.wantPreferred[i])
				}
			}
		})
	}
}
