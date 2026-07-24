package client

import (
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// NewFromProvider creates an API client from an EffectiveProvider.
// Routes to the appropriate client implementation based on provider type.
func NewFromProvider(p *provider.EffectiveProvider) *Client {
	switch p.Type {
	case types.ProviderOllama:
		return NewWithProvider("", p.BaseURL, p.HTTPProxy, types.ProviderOllama)
	case types.ProviderAnthropic:
		return NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderAnthropic)
	default:
		return NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderOpenAI)
	}
}
