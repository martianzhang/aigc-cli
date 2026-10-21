package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/webtool"
)

// The MCP web tools mirror the chat primitives exactly, so one mental model
// covers both surfaces (and the docs describe them once):
//
//	search / fetch           built-in, free, no API key
//	web_search / web_fetch   third-party provider, may bill per request
//
// Vendor resolution differs only in scope: MCP uses the shared
// `defaults.search.provider` default, while chat also honours its
// --search-provider flag.

// webEnv is the vendor-resolution environment for web_search / web_fetch.
func (cfg *Config) webEnv() webtool.Env {
	return webtool.Env{
		Provider:  cfg.SearchProvider,
		Providers: cfg.WebSearchProviders,
		Scope:     "defaults.search.provider",
	}
}

// argsJSON re-serializes a tool call's arguments for the webtool entry points,
// which take the raw JSON object the model produced.
func argsJSON(req mcp.CallToolRequest) (string, error) {
	args, err := argsMap(req)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	return string(raw), nil
}

// ----- Tool definitions -----

func newSearchTool() mcp.Tool {
	return mcp.NewTool("search",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Search the web with the built-in free engine (DuckDuckGo) and return ranked titles and URLs. Free, no API key. Read a result in full with `fetch`. Use `web_search` instead when a third-party search API is configured."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max results to return (default 5, max 10)"),
		),
	)
}

func newFetchTool() mcp.Tool {
	return mcp.NewTool("fetch",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Fetch a URL with the built-in free fetcher (direct HTTP; HTML is converted to markdown, main content only) and return its content. Non-HTML content is returned as-is. Free, no API key, SSRF-guarded. Large results are paginated by character offset. Use `web_fetch` to go through a configured third-party API instead."),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description("Absolute http(s) URL to fetch"),
		),
		mcp.WithNumber("max_length",
			mcp.Description("Max characters to return (default 5000, max 50000)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Character offset to start from (for pagination)"),
		),
	)
}

func newWebSearchTool() mcp.Tool {
	return mcp.NewTool("web_search",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Search the web through the configured third-party provider (defaults.search.provider) and return ranked titles and URLs. The provider may bill per request. Requires a provider under `web_search.*`. For free searching use `search`."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max results to return (default 5, max 10)"),
		),
	)
}

func newWebFetchTool() mcp.Tool {
	return mcp.NewTool("web_fetch",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription("Fetch a URL through the configured third-party fetch API (the same provider web_search uses) and return provider-formatted Markdown. The provider may bill per request. Requires a fetch-capable provider under `web_search.*`. For free fetching use `fetch`."),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description("Absolute http(s) URL to fetch"),
		),
		mcp.WithNumber("max_length",
			mcp.Description("Max characters to return (default 5000, max 50000)"),
		),
		mcp.WithNumber("offset",
			mcp.Description("Character offset to start from (for pagination)"),
		),
	)
}

// ----- Handlers -----

func searchHandler() server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := argsJSON(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(webtool.Search(args)), nil
	}
}

func fetchHandler() server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := argsJSON(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(webtool.Fetch(args)), nil
	}
}

func webSearchHandler(cfg *Config) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := argsJSON(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(webtool.WebSearch(args, cfg.webEnv())), nil
	}
}

func webFetchHandler(cfg *Config) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := argsJSON(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(webtool.WebFetch(args, cfg.webEnv())), nil
	}
}
