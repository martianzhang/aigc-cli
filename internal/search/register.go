package search

import (
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// NewRouterAll returns a router with every configured provider plus the
// built-in DuckDuckGo provider. It is the entry point for `search` and
// `kb search`.
func NewRouterAll(providers map[string]*types.WebSearchProvider) *Router {
	router := NewRouter()
	registerAll(router, providers)
	return router
}

// NewRemoteRouter returns a router with third-party providers only (no
// DuckDuckGo fallback). It is the entry point for chat/MCP web_search and
// web_fetch.
func NewRemoteRouter(providers map[string]*types.WebSearchProvider) *Router {
	router := NewRouter()
	registerRemote(router, providers)
	return router
}

// registerAll registers every configured web search provider and always
// ensures the built-in DuckDuckGo provider is available. Providers that cannot
// be constructed (e.g. a paid provider with a missing api_key) are recorded as
// unavailable with their reason, so errors can explain what is wrong.
func registerAll(router *Router, providers map[string]*types.WebSearchProvider) {
	registerConfigured(router, providers, false)
	if _, ok := router.GetProvider("duckduckgo"); !ok {
		if _, unavailable := router.Unavailable("duckduckgo"); !unavailable {
			router.RegisterWithType("duckduckgo", NewDDGProvider(), "duckduckgo")
		}
	}
}

// registerRemote registers only third-party providers — everything except the
// built-in DuckDuckGo scraper — and adds no fallback. NewRemoteRouter uses it
// so chat/MCP web_search never silently answer from the built-in engine that
// the `search` primitive already covers.
func registerRemote(router *Router, providers map[string]*types.WebSearchProvider) {
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
