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
	OpenLux
	ModelScope
	Agnes
	Gemini
	Bailian
	Zeekai
	Pollinations
)

var names = map[Type]string{
	Unknown:      "unknown",
	APIMart:      "APIMart",
	OpenAI:       "OpenAI",
	OpenRouter:   "OpenRouter",
	OpenLux:      "OpenLux",
	ModelScope:   "ModelScope",
	Agnes:        "Agnes",
	Gemini:       "Gemini",
	Bailian:      "阿里云百炼",
	Zeekai:       "ZeekAI",
	Pollinations: "Pollinations",
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
	return t == APIMart || t == ModelScope
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

// openluxDomains lists domains where OpenLux APIs are served.
var openluxDomains = []string{
	"openlux.ai",
}

// modelscopeDomains lists domains where ModelScope API-Inference is served.
var modelscopeDomains = []string{
	"api-inference.modelscope.cn",
	"api-inference.modelscope.ai",
}

// agnesDomains lists domains where Agnes AI APIs are served.
var agnesDomains = []string{
	"agnes-ai.com",
	"agnes-ai.cn",
}

// geminiDomains lists domains where Google Gemini APIs are served.
var geminiDomains = []string{
	"generativelanguage.googleapis.com",
}

// bailianDomains lists domains where Alibaba Cloud Bailian (DashScope) APIs are
// served. Covers the workspace-scoped native hosts used by DashScope-native
// models such as fun-music (e.g. {WorkspaceId}.cn-beijing.maas.aliyuncs.com) as
// well as the legacy and international DashScope endpoints.
var bailianDomains = []string{
	"maas.aliyuncs.com",
	"dashscope.aliyuncs.com",
	"dashscope-intl.aliyuncs.com",
}

// zeekaiDomains lists domains where ZeekAI relay APIs are served. ZeekAI
// rejects top-level image_urls on /images/generations and only accepts
// POST /images/edits with images[].image_url for image-to-image.
var zeekaiDomains = []string{
	"zeekai.cc",
}

// pollinationsDomains lists domains where pollinations.ai APIs are served.
// Its media generation endpoints (/image/{prompt}, /video/{prompt}) live at
// the API root rather than under the /v1 version prefix used by its
// OpenAI-compatible routes.
var pollinationsDomains = []string{
	"pollinations.ai",
}

// matchDomain checks that host is the domain d or a subdomain of d.
// Uses url.Parse + u.Host to compare domains accurately and avoid
// false positives like "x.evil.com" matching "evil.com".
func matchDomain(baseURL, d string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	return host == d || strings.HasSuffix(host, "."+d)
}

// Detect identifies the provider from an API base URL.
// Returns Unknown if the URL doesn't match any known provider.
func Detect(baseURL string) Type {
	if baseURL == "" {
		return Unknown
	}
	for _, d := range apimartDomains {
		if matchDomain(baseURL, d) {
			return APIMart
		}
	}
	for _, d := range openrouterDomains {
		if matchDomain(baseURL, d) {
			return OpenRouter
		}
	}
	for _, d := range openluxDomains {
		if matchDomain(baseURL, d) {
			return OpenLux
		}
	}
	for _, d := range modelscopeDomains {
		if matchDomain(baseURL, d) {
			return ModelScope
		}
	}
	for _, d := range agnesDomains {
		if matchDomain(baseURL, d) {
			return Agnes
		}
	}
	for _, d := range geminiDomains {
		if matchDomain(baseURL, d) {
			if strings.HasSuffix(baseURL, "/openai") || strings.HasSuffix(baseURL, "/openai/") {
				return OpenAI
			}
			return Gemini
		}
	}
	for _, d := range bailianDomains {
		if matchDomain(baseURL, d) {
			return Bailian
		}
	}
	for _, d := range zeekaiDomains {
		if matchDomain(baseURL, d) {
			return Zeekai
		}
	}
	for _, d := range pollinationsDomains {
		if matchDomain(baseURL, d) {
			return Pollinations
		}
	}
	// Default to OpenAI-compatible for everything else
	return OpenAI
}

// IsAPIMart is a convenience wrapper around Detect.
func IsAPIMart(baseURL string) bool { return Detect(baseURL) == APIMart }

// IsOpenRouter is a convenience wrapper around Detect.
func IsOpenRouter(baseURL string) bool { return Detect(baseURL) == OpenRouter }

// IsOpenLux is a convenience wrapper around Detect.
func IsOpenLux(baseURL string) bool { return Detect(baseURL) == OpenLux }

// IsModelScope is a convenience wrapper around Detect.
func IsModelScope(baseURL string) bool { return Detect(baseURL) == ModelScope }

// IsAgnes is a convenience wrapper around Detect.
func IsAgnes(baseURL string) bool { return Detect(baseURL) == Agnes }

// IsGemini is a convenience wrapper around Detect.
func IsGemini(baseURL string) bool { return Detect(baseURL) == Gemini }

// IsBailian is a convenience wrapper around Detect.
func IsBailian(baseURL string) bool { return Detect(baseURL) == Bailian }

// IsZeekai is a convenience wrapper around Detect.
func IsZeekai(baseURL string) bool { return Detect(baseURL) == Zeekai }

// IsPollinations is a convenience wrapper around Detect.
func IsPollinations(baseURL string) bool { return Detect(baseURL) == Pollinations }

// IsGeminiDomain returns true if baseURL points to Google Gemini API,
// including the OpenAI-compatible /openai endpoint variant.
// Unlike IsGemini, this catches URLs like
// generativelanguage.googleapis.com/v1beta/openai which Detect reports as OpenAI.
func IsGeminiDomain(baseURL string) bool {
	for _, d := range geminiDomains {
		if matchDomain(baseURL, d) {
			return true
		}
	}
	return false
}

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
