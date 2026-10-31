package client

import (
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// NewFromProvider creates an API client from an EffectiveProvider.
// Routes to the appropriate wire protocol based on the provider's configured
// `type`. Unknown / protocol-agnostic types fall back to the OpenAI-compatible
// chat-completions client, so new type values stay forward-compatible.
// The provider's ZDR preference is carried onto the client.
func NewFromProvider(p *provider.EffectiveProvider) *Client {
	var c *Client
	switch p.Type {
	case types.ProviderOllama:
		c = NewWithProvider("", p.BaseURL, p.HTTPProxy, types.ProviderOllama)
	case types.ProviderAnthropic:
		c = NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderAnthropic)
	case types.ProviderOpenAIResponses:
		c = NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderOpenAIResponses)
	case types.ProviderOpenAI, types.ProviderOpenAIChatCompletions, "":
		c = NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderOpenAI)
	default:
		c = NewWithProvider(p.APIKey, p.BaseURL, p.HTTPProxy, types.ProviderOpenAI)
	}
	c.zdr = p.ZDR
	// Honor an explicit --timeout / config timeout for every command, including
	// those that build a client directly from a resolved provider (MCP, agent
	// loops) instead of calling options.ApplyTimeout.
	if p.Timeout > 0 {
		c.SetTimeout(p.Timeout)
	}
	return c
}
