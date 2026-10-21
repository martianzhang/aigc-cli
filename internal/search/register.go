package search

import (
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// RegisterAll registers every configured web search provider and always
// ensures the built-in DuckDuckGo provider is available. Providers that cannot
// be constructed (e.g. a paid provider with a missing api_key) are recorded as
// unavailable with their reason, so errors can explain what is wrong.
func RegisterAll(router *Router, providers map[string]*types.WebSearchProvider) {
	registerConfigured(router, providers, false)
	if _, ok := router.GetProvider("duckduckgo"); !ok {
		if _, unavailable := router.Unavailable("duckduckgo"); !unavailable {
			router.RegisterWithType("duckduckgo", NewDDGProvider(), "duckduckgo")
		}
	}
}

// RegisterRemote registers only third-party providers — everything except the
// built-in DuckDuckGo scraper — and adds no fallback. Chat/MCP web_search use
// it so they never silently answer from the built-in engine that the `search`
// primitive already covers.
func RegisterRemote(router *Router, providers map[string]*types.WebSearchProvider) {
	registerConfigured(router, providers, true)
}

func registerConfigured(router *Router, providers map[string]*types.WebSearchProvider, remoteOnly bool) {
	for name, cfg := range providers {
		if cfg == nil {
			router.RegisterUnavailable(name, "", fmt.Errorf("empty provider config"))
			continue
		}
		if remoteOnly && cfg.Type == "duckduckgo" {
			continue
		}
		p, err := NewProviderFromConfig(cfg)
		if err != nil {
			router.RegisterUnavailable(name, cfg.Type, err)
			continue
		}
		router.RegisterWithType(name, p, cfg.Type)
	}
}
