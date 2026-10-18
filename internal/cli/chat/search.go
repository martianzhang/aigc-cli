package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/search"
)

const (
	// defaultWebSearchLimit is used when the model omits `limit`.
	defaultWebSearchLimit = 5
	// maxWebSearchLimit caps how many results a single call may request.
	maxWebSearchLimit = 10
)

// webSearchArgs are the arguments shared by the `search` and `web_search`
// primitives.
type webSearchArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// parseWebSearchArgs validates and normalizes the shared search arguments,
// returning a model-facing message on error.
func parseWebSearchArgs(argsJSON string) (webSearchArgs, string) {
	var args webSearchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return args, fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return args, "Error: query is required"
	}
	if args.Limit <= 0 {
		args.Limit = defaultWebSearchLimit
	}
	if args.Limit > maxWebSearchLimit {
		args.Limit = maxWebSearchLimit
	}
	return args, ""
}

// executeBuiltinSearch dispatches the `search` primitive — the built-in, free
// DuckDuckGo scrape. It calls no provider API and is never billed.
func executeBuiltinSearch(argsJSON string) string {
	args, errMsg := parseWebSearchArgs(argsJSON)
	if errMsg != "" {
		return errMsg
	}

	urls, err := knowledge.DDGSearchURLs(args.Query)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	results := make([]search.Result, 0, len(urls))
	for _, u := range urls {
		results = append(results, search.Result{URL: u, Title: u})
	}
	return formatSearchResults(args.Query, &search.SearchResult{Provider: "duckduckgo", Results: results}, "fetch")
}

// executeWebSearch dispatches the `web_search` primitive, which queries the
// configured third-party search provider (the same vendor web_fetch uses).
func executeWebSearch(argsJSON string) string {
	args, errMsg := parseWebSearchArgs(argsJSON)
	if errMsg != "" {
		return errMsg
	}

	router := remoteRouter()
	result, err := searchWithRouter(router, args.Query, args.Limit)
	if err != nil {
		return fmt.Sprintf("Error: %v. Configure a third-party provider under web_search.* (selected by defaults.chat.search_provider), or use the built-in `search` primitive.", err)
	}
	return formatSearchResults(args.Query, result, "web_fetch")
}

// remoteRouter builds a router over the configured third-party providers only.
// The built-in free DuckDuckGo engine is excluded on purpose: it is the `search`
// primitive's job, and including it would let `web_search` silently answer from
// the built-in engine under `auto`.
func remoteRouter() *search.Router {
	router := search.NewRouter(search.NoopQuotaStore{})
	search.RegisterRemote(router, options.WebSearchConfig())
	return router
}

// searchWithRouter picks a provider from the --search-provider flag (which
// overrides defaults.chat.search_provider) and runs one search.
func searchWithRouter(router *search.Router, query string, limit int) (*search.SearchResult, error) {
	strategy, preferred := search.ResolveStrategy(resolveSearchVendor())
	return router.Search(query, limit, strategy, preferred)
}

// resolveSearchVendor returns the provider spec shared by the web_search and
// web_fetch primitives — they target the same vendor, which bundles search and
// fetch. Precedence: --search-provider > defaults.chat.search_provider > auto.
func resolveSearchVendor() string {
	if chatSearchProvider != "" {
		return chatSearchProvider
	}
	if cfg := options.ChatDefaults(); cfg != nil && cfg.SearchProvider != "" {
		return cfg.SearchProvider
	}
	return "auto"
}

// formatSearchResults renders a SearchResult as a compact, LLM-friendly list.
// fetchTool is the primitive the model should use to read a result in full
// ("fetch" for the built-in search, "web_fetch" for the third-party one).
func formatSearchResults(query string, result *search.SearchResult, fetchTool string) string {
	if result == nil || len(result.Results) == 0 {
		return fmt.Sprintf("No search results for %q.", query)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Search results for %q (via %s):\n", query, result.Provider)
	for i, r := range result.Results {
		// Some engines (e.g. DuckDuckGo) return no title — the URL stands in
		// for it, so print the URL only once.
		if r.Title == "" || r.Title == r.URL {
			fmt.Fprintf(&b, "%d. %s\n", i+1, r.URL)
			continue
		}
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.Title, r.URL)
	}
	fmt.Fprintf(&b, "\nUse %s with a URL above to read a page in full.", fetchTool)
	return b.String()
}
