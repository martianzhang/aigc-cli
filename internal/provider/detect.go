// Package provider detects which AI provider the user has configured
// based on the API base URL. Centralizes provider detection so that
// adding a new provider only requires changes here and in strategy tables.
package provider

import (
	"net/url"
	"strings"
)

// Type represents the identified API provider.
type Type int

const (
	Unknown Type = iota
	APIMart
	OpenAI
	OpenRouter
	Yunwu
)

var names = map[Type]string{
	Unknown:    "unknown",
	APIMart:    "APIMart",
	OpenAI:     "OpenAI",
	OpenRouter: "OpenRouter",
	Yunwu:      "Yunwu（云雾AI）",
}

func (t Type) String() string {
	if s, ok := names[t]; ok {
		return s
	}
	return "unknown"
}

// IsAsync returns true if this provider uses an async task-based model
// (submit → poll → download) for generation.
func (t Type) IsAsync() bool {
	return t == APIMart
}

// apimartDomains lists known APIMart-provided API domains.
var apimartDomains = []string{
	"apimart.ai",
	"apib.ai",
	"aiuxu.com",
	"aishuch.com",
}

// openrouterDomains lists domains where OpenRouter APIs are served.
var openrouterDomains = []string{
	"openrouter.ai",
}

// yunwuDomains lists domains where Yunwu AI (云雾AI) APIs are served.
var yunwuDomains = []string{
	"yunwu.ai",
}

// Detect identifies the provider from an API base URL.
// Returns Unknown if the URL doesn't match any known provider.
func Detect(baseURL string) Type {
	if baseURL == "" {
		return Unknown
	}
	for _, d := range apimartDomains {
		if strings.Contains(baseURL, d) {
			return APIMart
		}
	}
	for _, d := range openrouterDomains {
		if strings.Contains(baseURL, d) {
			return OpenRouter
		}
	}
	for _, d := range yunwuDomains {
		if strings.Contains(baseURL, d) {
			return Yunwu
		}
	}
	// Default to OpenAI-compatible for everything else
	return OpenAI
}

// IsAPIMart is a convenience wrapper around Detect.
func IsAPIMart(baseURL string) bool { return Detect(baseURL) == APIMart }

// IsOpenRouter is a convenience wrapper around Detect.
func IsOpenRouter(baseURL string) bool { return Detect(baseURL) == OpenRouter }

// IsYunwu is a convenience wrapper around Detect.
func IsYunwu(baseURL string) bool { return Detect(baseURL) == Yunwu }

// localHostnames lists hostnames that are considered local/loopback addresses.
var localHostnames = map[string]bool{
	"localhost": true,
	"127.0.0.1": true,
	"::1":       true,
}

// IsLocalEndpoint returns true if the base URL points to a local/loopback address.
// Local model servers (Ollama, LM Studio, vLLM, etc.) are typically OpenAI-compatible
// but don't require API keys.
func IsLocalEndpoint(baseURL string) bool {
	if baseURL == "" {
		return false
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return localHostnames[strings.ToLower(u.Hostname())]
}
