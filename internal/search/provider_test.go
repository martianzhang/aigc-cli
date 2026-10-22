package search

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestNewProviderFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *types.WebSearchProvider
		wantErr bool
		check   func(t *testing.T, p Provider)
	}{
		{
			name: "duckduckgo returns DDGProvider",
			cfg:  &types.WebSearchProvider{Type: "duckduckgo"},
			check: func(t *testing.T, p Provider) {
				if _, ok := p.(*DDGProvider); !ok {
					t.Errorf("NewProviderFromConfig() = %T, want *DDGProvider", p)
				}
			},
		},
		{
			name:    "firecrawl without api key returns error",
			cfg:     &types.WebSearchProvider{Type: "firecrawl"},
			wantErr: true,
		},
		{
			name:    "brave without api key returns error",
			cfg:     &types.WebSearchProvider{Type: "brave"},
			wantErr: true,
		},
		{
			name:    "doubao without api key returns error",
			cfg:     &types.WebSearchProvider{Type: "doubao"},
			wantErr: true,
		},
		{
			name:    "deepseek without api key returns error",
			cfg:     &types.WebSearchProvider{Type: "deepseek"},
			wantErr: true,
		},
		{
			name:    "exa without api key returns error",
			cfg:     &types.WebSearchProvider{Type: "exa"},
			wantErr: true,
		},
		{
			name:    "pixabay without api key returns error",
			cfg:     &types.WebSearchProvider{Type: "pixabay"},
			wantErr: true,
		},
		{
			name:    "pexels without api key returns error",
			cfg:     &types.WebSearchProvider{Type: "pexels"},
			wantErr: true,
		},
		{
			name:    "unknown type returns error",
			cfg:     &types.WebSearchProvider{Type: "nonexistent"},
			wantErr: true,
		},
		{
			name: "firecrawl with api key returns FirecrawlProvider",
			cfg:  &types.WebSearchProvider{Type: "firecrawl", APIKey: "fc-key"},
			check: func(t *testing.T, p Provider) {
				if _, ok := p.(*FirecrawlProvider); !ok {
					t.Errorf("NewProviderFromConfig() = %T, want *FirecrawlProvider", p)
				}
			},
		},
		{
			name: "brave with api key returns BraveProvider",
			cfg:  &types.WebSearchProvider{Type: "brave", APIKey: "brave-key"},
			check: func(t *testing.T, p Provider) {
				if _, ok := p.(*BraveProvider); !ok {
					t.Errorf("NewProviderFromConfig() = %T, want *BraveProvider", p)
				}
			},
		},
		{
			name: "doubao with api key returns DoubaoProvider",
			cfg:  &types.WebSearchProvider{Type: "doubao", APIKey: "doubao-key"},
			check: func(t *testing.T, p Provider) {
				if _, ok := p.(*DoubaoProvider); !ok {
					t.Errorf("NewProviderFromConfig() = %T, want *DoubaoProvider", p)
				}
			},
		},
		{
			name: "deepseek with api key returns DeepSeekProvider",
			cfg:  &types.WebSearchProvider{Type: "deepseek", APIKey: "ds-key"},
			check: func(t *testing.T, p Provider) {
				if _, ok := p.(*DeepSeekProvider); !ok {
					t.Errorf("NewProviderFromConfig() = %T, want *DeepSeekProvider", p)
				}
			},
		},
		{
			name: "exa with api key returns ExaProvider",
			cfg:  &types.WebSearchProvider{Type: "exa", APIKey: "exa-key"},
			check: func(t *testing.T, p Provider) {
				if _, ok := p.(*ExaProvider); !ok {
					t.Errorf("NewProviderFromConfig() = %T, want *ExaProvider", p)
				}
			},
		},
		{
			name: "pixabay with api key returns PixabayProvider",
			cfg:  &types.WebSearchProvider{Type: "pixabay", APIKey: "pixabay-key"},
			check: func(t *testing.T, p Provider) {
				if _, ok := p.(*PixabayProvider); !ok {
					t.Errorf("NewProviderFromConfig() = %T, want *PixabayProvider", p)
				}
			},
		},
		{
			name: "pexels with api key returns PexelsProvider",
			cfg:  &types.WebSearchProvider{Type: "pexels", APIKey: "pexels-key"},
			check: func(t *testing.T, p Provider) {
				if _, ok := p.(*PexelsProvider); !ok {
					t.Errorf("NewProviderFromConfig() = %T, want *PexelsProvider", p)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure ambient keys cannot leak into the "without api key" cases.
			t.Setenv("FIRECRAWL_API_KEY", "")
			t.Setenv("EXA_API_KEY", "")
			t.Setenv("PIXABAY_API_KEY", "")
			t.Setenv("PEXELS_API_KEY", "")

			p, err := NewProviderFromConfig(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewProviderFromConfig() error = nil, want error")
				}
				if p != nil {
					t.Errorf("NewProviderFromConfig() provider = %T, want nil", p)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewProviderFromConfig() unexpected error: %v", err)
			}
			if p == nil {
				t.Fatalf("NewProviderFromConfig() provider = nil, want non-nil")
			}
			if tt.check != nil {
				tt.check(t, p)
			}
		})
	}
}

func TestNewProviderFromConfigEnvKeys(t *testing.T) {
	t.Setenv("PIXABAY_API_KEY", "env-pb-key")
	t.Setenv("PEXELS_API_KEY", "env-px-key")

	for _, tc := range []struct {
		typ  string
		want string
	}{
		{typ: "pixabay", want: "pixabay"},
		{typ: "pexels", want: "pexels"},
	} {
		p, err := NewProviderFromConfig(&types.WebSearchProvider{Type: tc.typ})
		if err != nil {
			t.Fatalf("%s: NewProviderFromConfig() error = %v", tc.typ, err)
		}
		if p.Name() != tc.want {
			t.Errorf("%s: Name() = %q, want %q", tc.typ, p.Name(), tc.want)
		}
	}
}
