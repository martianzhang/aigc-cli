package search

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestRegisterAllRecordsUnavailable(t *testing.T) {
	r := NewRouter()
	RegisterAll(r, map[string]*types.WebSearchProvider{
		"brave":   {Type: "brave"}, // missing api_key
		"mystery": {Type: "mystery"},
	})

	if _, ok := r.Unavailable("brave"); !ok {
		t.Error("brave should be recorded as unavailable")
	}
	if _, ok := r.Unavailable("mystery"); !ok {
		t.Error("unknown provider type should be recorded as unavailable")
	}

	statuses := r.Statuses()
	foundBrave := false
	for _, status := range statuses {
		if status.Name == "brave" {
			foundBrave = true
			if status.Available || !strings.Contains(status.Err, "api_key") {
				t.Errorf("brave status = %+v, want unavailable with api_key reason", status)
			}
		}
	}
	if !foundBrave {
		t.Error("brave missing from Statuses()")
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
