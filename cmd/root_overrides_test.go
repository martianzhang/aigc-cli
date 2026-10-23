package cmd

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestCmdProviderWarnings(t *testing.T) {
	providers := map[string]*types.NamedProvider{"main": {}}

	cases := []struct {
		name     string
		cfg      *types.Config
		wantWarn bool
	}{
		{
			name: "local sentinel is valid",
			cfg: &types.Config{Defaults: &types.ConfigDefaults{
				Audio: &types.AudioDefaults{SFX: &types.AudioSFXDefaults{Provider: "local"}},
			}},
		},
		{
			name: "named provider is valid",
			cfg: &types.Config{
				Providers: providers,
				Defaults: &types.ConfigDefaults{
					Image: &types.ImageDefaults{Provider: "main"},
				},
			},
		},
		{
			name: "unknown provider warns",
			cfg: &types.Config{Defaults: &types.ConfigDefaults{
				Image: &types.ImageDefaults{Provider: "missing"},
			}},
			wantWarn: true,
		},
		{
			name: "nil config",
			cfg:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warnings := cmdProviderWarnings(tc.cfg)
			if tc.wantWarn && len(warnings) == 0 {
				t.Fatal("expected a warning")
			}
			if !tc.wantWarn && len(warnings) != 0 {
				t.Fatalf("unexpected warnings: %v", warnings)
			}
			for _, w := range warnings {
				if !strings.Contains(w, "provider") {
					t.Errorf("warning = %q, want it to mention provider", w)
				}
			}
		})
	}
}
