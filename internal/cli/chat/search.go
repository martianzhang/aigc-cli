package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/search"
)

const (
	// defaultWebSearchLimit is used when the model omits `limit`.
	defaultWebSearchLimit = 5
	// maxWebSearchLimit caps how many results a single call may request.
	maxWebSearchLimit = 10
)

// webSearchArgs are the arguments for the `web_search` primitive.
type webSearchArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// executeWebSearch is the dispatcher for the `web_search` primitive. It searches
// the web via the configured providers (top-level `web_search.*`) and returns a
// compact list of titles and URLs for the model to read with `web_fetch`.
func executeWebSearch(argsJSON string) string {
	var args webSearchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return "Error: query is required"
	}

	limit := args.Limit
	if limit <= 0 {
		limit = defaultWebSearchLimit
	}
	if limit > maxWebSearchLimit {
		limit = maxWebSearchLimit
	}

	result, err := runWebSearch(args.Query, limit)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return formatSearchResults(args.Query, result)
}

// runWebSearch builds a router from the configured web_search providers and
// performs one search. Quota tracking is disabled because chat has no knowledge
// base database open.
func runWebSearch(query string, limit int) (*search.SearchResult, error) {
	router := search.NewRouter(search.NoopQuotaStore{})
	search.RegisterAll(router, options.WebSearchConfig())
	return searchWithRouter(router, query, limit)
}

// searchWithRouter picks a strategy from the --search-provider flag (which
// overrides defaults.chat.search_provider) and runs one search, preferring a
// named provider when one is configured.
func searchWithRouter(router *search.Router, query string, limit int) (*search.SearchResult, error) {
	provider := chatSearchProvider
	if provider == "" {
		if cfg := options.ChatDefaults(); cfg != nil {
			provider = cfg.SearchProvider
		}
	}
	strategy, preferred := search.ResolveStrategy(provider)
	return router.Search(query, limit, strategy, preferred)
}

// formatSearchResults renders a SearchResult as a compact, LLM-friendly list.
func formatSearchResults(query string, result *search.SearchResult) string {
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
	b.WriteString("\nUse web_fetch with a URL above to read a page in full.")
	return b.String()
}
