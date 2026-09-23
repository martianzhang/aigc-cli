package provider

import (
	"net/http"
	"os"
)

// Version mirrors client.Version and is copied in at startup (cmd/root.go), so
// every outbound LLM request can report the same User-Agent.
var Version = "dev"

// OpenRouter app-attribution header names. HTTP-Referer is required for an app
// to be attributed at all; X-OpenRouter-Title names it.
const (
	HeaderReferer    = "HTTP-Referer"
	HeaderTitle      = "X-OpenRouter-Title"
	HeaderCategories = "X-OpenRouter-Categories"
)

// Built-in attribution identity, used when the OPENAI_REFERER / OPENAI_APP_TITLE
// env vars are unset, so the tool appears in OpenRouter rankings by name
// instead of as an anonymous client.
const (
	DefaultReferer    = "https://github.com/martianzhang/aigc-cli"
	DefaultTitle      = "aigc-cli"
	DefaultCategories = "cli-agent"
)

// SetAttribution identifies aigc-cli on an outbound LLM request: it always sets
// the User-Agent and, when baseURL is OpenRouter, the app-attribution headers.
// It mirrors client.setOpenRouterHeaders, which the provider package cannot
// call because client imports provider and not the other way round.
func SetAttribution(req *http.Request, baseURL string) {
	req.Header.Set("User-Agent", "aigc-cli/"+Version)

	referer, title := "", ""
	if IsOpenRouter(baseURL) {
		referer, title = DefaultReferer, DefaultTitle
		req.Header.Set(HeaderCategories, DefaultCategories)
	}
	if v := os.Getenv("OPENAI_REFERER"); v != "" {
		referer = v
	}
	if v := os.Getenv("OPENAI_APP_TITLE"); v != "" {
		title = v
	}
	if referer != "" {
		req.Header.Set(HeaderReferer, referer)
	}
	if title != "" {
		req.Header.Set(HeaderTitle, title)
	}
}
