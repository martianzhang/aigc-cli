// Package webtool implements the shared search/fetch tool layer used by the
// chat primitives and the MCP server. It deliberately owns no configuration
// lookup: callers pass in the vendor spec and the provider registry, so the
// same implementation serves every entry point (chat, MCP, …).
//
// Naming follows one rule everywhere (see docs/*/guide-chat.md):
//
//	search / fetch           built-in, free, no API key
//	web_search / web_fetch   third-party provider, may bill per request
package webtool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	// DefaultSearchLimit is used when the model omits `limit`.
	DefaultSearchLimit = 5
	// MaxSearchLimit caps how many results a single call may request.
	MaxSearchLimit = 10
)

// searchArgs are the arguments shared by the `search` and `web_search` tools.
type searchArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// parseSearchArgs validates and normalizes the shared search arguments,
// returning a model-facing message on error.
func parseSearchArgs(argsJSON string) (searchArgs, string) {
	var args searchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return args, fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return args, "Error: query is required"
	}
	if args.Limit <= 0 {
		args.Limit = DefaultSearchLimit
	}
	if args.Limit > MaxSearchLimit {
		args.Limit = MaxSearchLimit
	}
	return args, ""
}

// Search runs the built-in, free DuckDuckGo scrape. It calls no provider API
// and is never billed.
func Search(argsJSON string) string {
	args, errMsg := parseSearchArgs(argsJSON)
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
	return FormatSearchResults(args.Query, &search.SearchResult{Provider: "duckduckgo", Results: results}, "fetch")
}

// WebSearch runs a search against the third-party provider selected by env.
// It never falls back to the built-in DuckDuckGo engine: `search` already
// covers that, and a silent fallback would hide that no paid vendor ran.
func WebSearch(argsJSON string, env Env) string {
	args, errMsg := parseSearchArgs(argsJSON)
	if errMsg != "" {
		return errMsg
	}
	if strings.TrimSpace(env.Provider) == "" {
		return fmt.Sprintf("Error: no third-party provider configured. Set a provider name via %s or use the built-in `search` tool.", env.specHint())
	}

	router := env.router()
	result, err := router.Search(args.Query, args.Limit, env.Provider, search.Options{})
	if err != nil {
		return fmt.Sprintf("Error: %v. Configure the provider under web_search.* (selected by %s), or use the built-in `search` tool.",
			err, env.specHint())
	}
	return FormatSearchResults(args.Query, result, "web_fetch")
}

// FormatSearchResults renders a SearchResult as a compact, LLM-friendly list.
// fetchTool is the tool the model should use to read a result in full ("fetch"
// for the built-in search, "web_fetch" for the third-party one).
func FormatSearchResults(query string, result *search.SearchResult, fetchTool string) string {
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

// Env carries the vendor resolution inputs for the third-party tools. It keeps
// the tools free of any config/scope lookup so chat and MCP can each resolve
// their own scope and pass the result down.
type Env struct {
	// Provider is the configured vendor name. Empty means no third-party
	// search/fetch vendor is configured.
	Provider string
	// Providers is the web_search.* registry from config.
	Providers map[string]*types.WebSearchProvider
	// Scope names where Provider came from, for actionable error messages
	// (e.g. "defaults.search.provider"). Optional.
	Scope string
}

// router builds a router over the configured third-party providers only. The
// built-in DuckDuckGo engine is excluded on purpose: it is the `search` tool's
// job, and third-party tools must not silently answer from it.
func (e Env) router() *search.Router {
	router := search.NewRouter()
	search.RegisterRemote(router, e.Providers)
	return router
}

func (e Env) specHint() string {
	if e.Scope != "" {
		return e.Scope
	}
	return "defaults.search.provider"
}
