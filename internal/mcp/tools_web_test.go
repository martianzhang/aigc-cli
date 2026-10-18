package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestSpecOrAuto(t *testing.T) {
	if got := specOrAuto(""); got != "auto" {
		t.Errorf("specOrAuto(\"\") = %q, want auto", got)
	}
	if got := specOrAuto("  "); got != "auto" {
		t.Errorf("specOrAuto(blank) = %q, want auto", got)
	}
	if got := specOrAuto("brave"); got != "brave" {
		t.Errorf("specOrAuto(brave) = %q, want brave", got)
	}
}

func TestWebEnv_usesGlobalScope(t *testing.T) {
	cfg := &Config{}
	env := cfg.webEnv()
	if env.Spec != "auto" {
		t.Errorf("Spec = %q, want auto", env.Spec)
	}
	if env.Scope != "search_provider" {
		t.Errorf("Scope = %q, want search_provider", env.Scope)
	}

	providers := map[string]*types.WebSearchProvider{"brave": {Type: "brave", APIKey: "k"}}
	cfg = &Config{SearchProvider: "brave", WebSearchProviders: providers}
	env = cfg.webEnv()
	if env.Spec != "brave" || env.Providers["brave"] == nil {
		t.Errorf("env = %+v, want brave spec with providers wired", env)
	}
}

// With no third-party provider configured, web_search must explain which knob to
// set instead of silently answering from the built-in engine.
func TestWebSearchHandler_noProviderHints(t *testing.T) {
	handler := webSearchHandler(&Config{})
	res, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "web_search", Arguments: map[string]any{"query": "hi"}},
	})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	got := resultText(t, res)
	if !strings.Contains(got, "third-party provider") {
		t.Errorf("web_search result = %q, want a hint about configuring a provider", got)
	}
	if !strings.Contains(got, "search_provider") {
		t.Errorf("web_search result = %q, want the config key", got)
	}
}

func TestWebFetchHandler_noProviderHints(t *testing.T) {
	handler := webFetchHandler(&Config{})
	res, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "web_fetch", Arguments: map[string]any{"url": "https://example.com/a"}},
	})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	got := resultText(t, res)
	if !strings.Contains(got, "no third-party fetch provider") {
		t.Errorf("web_fetch result = %q, want a fetch-provider hint", got)
	}
}

func TestSearchHandler_missingQuery(t *testing.T) {
	handler := searchHandler()
	res, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "search", Arguments: map[string]any{}},
	})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if got := resultText(t, res); !strings.Contains(got, "query is required") {
		t.Errorf("search result = %q, want 'query is required'", got)
	}
}

func TestFetchHandler_missingURL(t *testing.T) {
	handler := fetchHandler()
	res, err := handler(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "fetch", Arguments: map[string]any{}},
	})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if got := resultText(t, res); !strings.Contains(got, "url is required") {
		t.Errorf("fetch result = %q, want 'url is required'", got)
	}
}

// The registry must expose all four web tools so the naming rule
// (search/fetch free, web_search/web_fetch third-party) holds in MCP too.
func TestToolRegistry_hasWebTools(t *testing.T) {
	for _, name := range []string{"search", "fetch", "web_search", "web_fetch"} {
		if registryTool(name) == nil {
			t.Errorf("tool registry is missing %q", name)
		}
	}
}
