package options

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestSearchProviderFromConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  *types.Config
		want string
	}{
		{name: "nil config", cfg: nil, want: ""},
		{
			name: "defaults.search.provider",
			cfg: &types.Config{Defaults: &types.ConfigDefaults{
				Search: &types.SearchDefaults{Provider: "deepseek"},
			}},
			want: "deepseek",
		},
		{
			name: "empty search section",
			cfg:  &types.Config{Defaults: &types.ConfigDefaults{Search: &types.SearchDefaults{}}},
			want: "",
		},
		{name: "nothing configured", cfg: &types.Config{}, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SearchProviderFromConfig(tc.cfg); got != tc.want {
				t.Errorf("SearchProviderFromConfig() = %q, want %q", got, tc.want)
			}
		})
	}
}
