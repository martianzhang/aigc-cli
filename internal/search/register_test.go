package search

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

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
			name: "configured provider plus duckduckgo",
			providers: map[string]*types.WebSearchProvider{
				"firecrawl": {Type: "firecrawl", APIKey: "fc-key"},
			},
			wantNames: []string{"firecrawl", "duckduckgo"},
		},
		{
			name: "explicit duckduckgo is not duplicated",
			providers: map[string]*types.WebSearchProvider{
				"duckduckgo": {Type: "duckduckgo"},
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
			r := NewRouter()
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
