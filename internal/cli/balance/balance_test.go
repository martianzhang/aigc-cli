package balance

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

func TestProviderLabel(t *testing.T) {
	if got := providerLabel(&provider.EffectiveProvider{Name: "siliconflow"}); got != "siliconflow" {
		t.Errorf("providerLabel(named) = %q, want siliconflow", got)
	}
	got := providerLabel(&provider.EffectiveProvider{ProviderType: provider.APIMart})
	if got != provider.APIMart.String() {
		t.Errorf("providerLabel(typed) = %q, want %q", got, provider.APIMart.String())
	}
}

func TestCollectProviders(t *testing.T) {
	t.Run("none configured", func(t *testing.T) {
		if got := collectProviders(Deps{}); got != nil {
			t.Errorf("collectProviders() = %v, want nil", got)
		}
	})

	t.Run("explicit provider", func(t *testing.T) {
		want := &provider.EffectiveProvider{Name: "p1"}
		got := collectProviders(Deps{
			ProviderSet:     true,
			Provider:        "p1",
			ResolveProvider: func(string) *provider.EffectiveProvider { return want },
		})
		if len(got) != 1 || got[0] != want {
			t.Errorf("collectProviders() = %v, want [p1]", got)
		}
	})
}
