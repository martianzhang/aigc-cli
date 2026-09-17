package options

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

func TestIsAPIMartProvider_ModeOverride(t *testing.T) {
	tests := []struct {
		name string
		mode string
		p    *provider.EffectiveProvider
		want bool
	}{
		{"mode async forces APIMart", "async", &provider.EffectiveProvider{BaseURL: "https://openrouter.ai/api/v1", ProviderType: provider.OpenRouter}, true},
		{"mode sync disables APIMart", "sync", &provider.EffectiveProvider{BaseURL: "https://api.apimart.ai", ProviderType: provider.APIMart}, false},
		{"auto detects APIMart vendor", "", &provider.EffectiveProvider{BaseURL: "https://api.apimart.ai", ProviderType: provider.APIMart}, true},
		{"auto detects APIMart from base URL", "", &provider.EffectiveProvider{BaseURL: "https://api.apimart.ai"}, true},
		{"auto rejects non-APIMart", "", &provider.EffectiveProvider{BaseURL: "https://openrouter.ai/api/v1", ProviderType: provider.OpenRouter}, false},
		{"nil provider falls back to default base", "", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer setSharedForTest(&SharedConfig{Mode: tc.mode})()
			if got := IsAPIMartProvider(tc.p); got != tc.want {
				t.Errorf("IsAPIMartProvider() = %v, want %v", got, tc.want)
			}
		})
	}
}
