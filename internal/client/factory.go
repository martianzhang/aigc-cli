package client

import "github.com/martianzhang/aigc-cli/internal/provider"

// NewFromProvider creates an API client from an EffectiveProvider.
// Routes to the appropriate client implementation based on provider type:
//
//   - openai (default) — OpenAI-compatible API
//   - ollama           — OpenAI-compatible client, no API key sent
//   - google           — Google Gemini API  (TODO: not implemented)
//   - anthropic        — Anthropic Messages API (TODO: not implemented)
func NewFromProvider(p *provider.EffectiveProvider) *Client {
	switch p.Type {
	case "ollama":
		return New("", p.BaseURL, p.HTTPProxy)
	case "google", "anthropic":
		fallthrough
	default:
		return New(p.APIKey, p.BaseURL, p.HTTPProxy)
	}
}
