package search

import (
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// RegisterAll registers every configured web search provider on the router and
// always ensures a free DuckDuckGo fallback, so a search never fails for lack
// of a provider. Providers that cannot be constructed (e.g. a paid provider
// with a missing api_key) are skipped, leaving the remaining ones usable.
func RegisterAll(router *Router, providers map[string]*types.WebSearchProvider) {
	configs := ConfigFromTypes(providers)
	for name, cfg := range providers {
		if cfg == nil {
			continue
		}
		p, err := NewProviderFromConfig(cfg)
		if err != nil {
			continue
		}
		router.Register(name, p, configs[name])
	}

	if _, ok := router.GetProvider("duckduckgo"); !ok {
		router.Register("duckduckgo", NewDDGProvider(), &ProviderInfo{
			Type:   "duckduckgo",
			Tags:   []string{"free"},
			Weight: 1,
		})
	}
}

// ResolveStrategy maps a provider spec to a router strategy and, for a named
// provider, the preferred list. It mirrors the values accepted by
// `kb search --provider` and `defaults.chat.search_provider`:
//
//	"" | "auto"      -> auto
//	"free" | "cheap" -> cheap
//	"quality"        -> quality
//	<any other name> -> manual, preferring that single provider
func ResolveStrategy(provider string) (strategy string, preferred []string) {
	p := strings.TrimSpace(provider)
	switch strings.ToLower(p) {
	case "", "auto":
		return "auto", nil
	case "free", "cheap":
		return "cheap", nil
	case "quality":
		return "quality", nil
	default:
		return "manual", []string{p}
	}
}
