package search

import (
	"reflect"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestConfigFromTypes(t *testing.T) {
	tests := []struct {
		name      string
		providers map[string]*types.WebSearchProvider
		wantNil   bool
		want      map[string]*ProviderInfo
	}{
		{
			name:      "nil map returns nil",
			providers: nil,
			wantNil:   true,
		},
		{
			name:      "empty map returns empty map",
			providers: map[string]*types.WebSearchProvider{},
			want:      map[string]*ProviderInfo{},
		},
		{
			name: "populated providers map all fields",
			providers: map[string]*types.WebSearchProvider{
				"brave": {
					Type:   "brave",
					APIKey: "brave-key",
					Tags:   []string{"free", "fast"},
					Quota:  100,
					Period: "monthly",
				},
				"duckduckgo": {
					Type: "duckduckgo",
					Tags: []string{"free"},
				},
			},
			want: map[string]*ProviderInfo{
				"brave": {
					Type:   "brave",
					APIKey: "brave-key",
					Tags:   []string{"free", "fast"},
					Quota:  100,
					Period: "monthly",
				},
				"duckduckgo": {
					Type: "duckduckgo",
					Tags: []string{"free"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConfigFromTypes(tt.providers)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("ConfigFromTypes() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ConfigFromTypes() = nil, want non-nil map")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConfigFromTypes() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDefaultConfigs(t *testing.T) {
	got := DefaultConfigs()
	if len(got) != 1 {
		t.Fatalf("DefaultConfigs() has %d providers, want 1", len(got))
	}
	ddg, ok := got["duckduckgo"]
	if !ok {
		t.Fatalf("DefaultConfigs() missing %q provider", "duckduckgo")
	}
	want := &ProviderInfo{
		Type:   "duckduckgo",
		Tags:   []string{"free"},
		Quota:  0,
		Period: "",
	}
	if !reflect.DeepEqual(ddg, want) {
		t.Errorf("DefaultConfigs()[duckduckgo] = %+v, want %+v", ddg, want)
	}
}
