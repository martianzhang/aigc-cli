package client

import (
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// NewFromProvider creates an API client from an EffectiveProvider.
// Routes to the appropriate wire protocol based on the provider's configured
// `type`. Unknown / protocol-agnostic types fall back to the OpenAI-compatible
// chat-completions client, so new type values stay forward-compatible.
func NewFromProvider(p *provider.EffectiveProvider) *Client {
	switch p.Type {
	case types.ProviderOllama:
		return NewWithProvider("", p.BaseURL, p.HTTPProxy, types.ProviderOllama)
	case types.ProviderAnthropic:
		return NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderAnthropic)
	case types.ProviderOpenAIResponses:
		return NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderOpenAIResponses)
	case types.ProviderOpenAI, types.ProviderOpenAIChatCompletions, "":
		return NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderOpenAI)
	default:
		return NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderOpenAI)
	}
}
