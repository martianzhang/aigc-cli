package chat

import (
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/webtool"
)

// The chat search/fetch primitives are thin wrappers over internal/webtool, so
// the same implementation also backs the MCP server. Only the vendor-spec
// resolution (which is CLI/config-scoped) lives here.
//
// Naming rule (shared with MCP): `search`/`fetch` are the built-in free tools;
// `web_search`/`web_fetch` go through the configured third-party provider and
// may bill per request.

// executeBuiltinSearch dispatches the `search` primitive — the built-in, free
// DuckDuckGo scrape. It calls no provider API and is never billed.
func executeBuiltinSearch(argsJSON string) string {
	return webtool.Search(argsJSON)
}

// executeWebSearch dispatches the `web_search` primitive, which queries the
// configured third-party search provider (the same vendor web_fetch uses).
func executeWebSearch(argsJSON string) string {
	return webtool.WebSearch(argsJSON, searchEnv())
}

// executeBuiltinFetch dispatches the `fetch` primitive: a free, SSRF-guarded
// direct HTTP GET converted to markdown.
func executeBuiltinFetch(argsJSON string) string {
	return webtool.Fetch(argsJSON)
}

// executeWebFetch dispatches the `web_fetch` primitive: the configured
// third-party fetch API (the same provider web_search uses).
func executeWebFetch(argsJSON string) string {
	return webtool.WebFetch(argsJSON, searchEnv())
}

// searchEnv builds the vendor-resolution environment for the third-party
// primitives. web_search and web_fetch share one vendor, which bundles both
// capabilities.
func searchEnv() webtool.Env {
	return webtool.Env{
		Provider:  resolveSearchVendor(),
		Providers: options.WebSearchConfig(),
		Scope:     "defaults.search.provider",
	}
}

// resolveSearchVendor returns the explicitly configured vendor for the
// web_search/web_fetch primitives: --search-provider > defaults.search.provider.
// An empty result means no third-party vendor is configured.
func resolveSearchVendor() string {
	if chatSearchProvider != "" {
		return chatSearchProvider
	}
	return options.SearchProviderDefault()
}
