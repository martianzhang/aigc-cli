package provider

import (
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/types"
)

const defaultBaseURL = "https://api.apimart.ai"

// EffectiveProvider is the resolved effective provider configuration for a command.
// It merges CLI flags, named provider config, global config, and built-in defaults.
// Commands use this directly — they should not access SharedConfig.APIKey/APIBase.
type EffectiveProvider struct {
	// Name is the named provider key (empty = global fallback).
	Name string
	// Type is the API protocol type (openai, ollama, google, anthropic, local).
	Type types.ProviderType
	// APIKey is the resolved API key.
	APIKey string
	// BaseURL is the resolved API base URL.
	BaseURL string
	// HTTPProxy is the resolved HTTP proxy.
	HTTPProxy string
	// ModelsDir is the local models directory (type=local).
	ModelsDir string
	// Model is the resolved model name/identifier.
	Model string
	// ProviderType is the cached provider type detection from BaseURL.
	ProviderType Type
}

// CLIOverride holds values from CLI flags (--api-key, --api-base, --http-proxy).
// These have the highest priority and skip all config-based resolution.
type CLIOverride struct {
	APIKey  string
	BaseURL string
	Proxy   string
	Model   string
}

// GlobalConfig holds the global fallback values from config.yaml top-level.
type GlobalConfig struct {
	APIKey  string
	BaseURL string
	Proxy   string
}

// ResolveCmdProvider resolves a command's effective provider by merging
// CLI flags, named provider config, global config, and built-in defaults.
//
// Priority (highest to lowest):
//  1. CLIOverride (--api-key / --api-base) — if non-empty, skip all config
//  2. defaults.{cmd}.provider → providers.{name}
//  3. GlobalConfig (top-level api_key / base_url)
//  4. Built-in defaults (APIMart base URL, empty API key)
//
// providerRef is the value of defaults.{cmd}.provider (e.g. "my-openrouter").
// namedProviders is the full providers map from config.
func ResolveCmdProvider(
	cli *CLIOverride,
	providerRef string,
	namedProviders map[string]*types.NamedProvider,
	global *GlobalConfig,
) *EffectiveProvider {
	// 1. CLI override — if explicitly set, skip all config
	if cli != nil && (cli.APIKey != "" || cli.BaseURL != "") {
		baseURL := cli.BaseURL
		if baseURL == "" {
			baseURL = firstNonEmpty(global.BaseURL, defaultBaseURL)
		}
		return &EffectiveProvider{
			APIKey:       cli.APIKey,
			BaseURL:      baseURL,
			HTTPProxy:    firstNonEmpty(cli.Proxy, global.Proxy),
			Model:        cli.Model,
			Type:         types.ProviderOpenAI,
			ProviderType: Detect(baseURL),
		}
	}

	// 2. Named provider
	if providerRef != "" && namedProviders != nil {
		if named, ok := namedProviders[providerRef]; ok {
			baseURL := named.BaseURL
			if baseURL == "" {
				baseURL = firstNonEmpty(global.BaseURL, defaultBaseURL)
			}
			ep := &EffectiveProvider{
				Name:         providerRef,
				Type:         named.Type,
				BaseURL:      baseURL,
				HTTPProxy:    firstNonEmpty(named.HTTPProxy, global.Proxy),
				ModelsDir:    named.ModelsDir,
				Model:        named.Model,
				ProviderType: Detect(baseURL),
			}
			if ep.Type == "" {
				ep.Type = types.ProviderOpenAI
			}
			// API key: named provider > global
			if named.APIKey != "" {
				ep.APIKey = named.APIKey
			} else {
				ep.APIKey = global.APIKey
			}
			// Provider type detection: only analyze base URL for types that
			// may route through an intermediary (OpenRouter, APIMart, etc.).
			if !ep.Type.DetectProvider() {
				ep.ProviderType = Unknown
			}
			return ep
		}
	}

	// 3. Global fallback
	baseURL := firstNonEmpty(global.BaseURL, defaultBaseURL)
	return &EffectiveProvider{
		APIKey:       global.APIKey,
		BaseURL:      baseURL,
		HTTPProxy:    global.Proxy,
		Type:         types.ProviderOpenAI,
		ProviderType: Detect(baseURL),
	}
}

// ValidateProviderRef checks that a provider reference name exists in the
// named providers map. Returns nil if ref is empty (means "use global").
func ValidateProviderRef(ref string, providers map[string]*types.NamedProvider) error {
	if ref == "" {
		return nil
	}
	if providers == nil {
		return fmt.Errorf("provider %q not found: no providers configured", ref)
	}
	if _, ok := providers[ref]; !ok {
		return fmt.Errorf("provider %q not found in config.providers", ref)
	}
	return nil
}

// IsOnlineProvider returns true if the provider should use an online API
// (as opposed to local ONNX inference). False for nil, local-only providers,
// or providers without a configured base URL.
//
// The decision is:
//   - Has a named provider reference (configured by user)
//   - OR explicitly typed as ollama
//   - OR points to a local/loopback endpoint (localhost, 127.0.0.1, etc.)
//
// Local-only providers (type=local) always return false regardless of base URL.
func IsOnlineProvider(p *EffectiveProvider) bool {
	return p != nil && p.BaseURL != "" && p.Type != types.ProviderLocal &&
		(p.Name != "" || p.Type == types.ProviderOllama || IsLocalEndpoint(p.BaseURL))
}

// RequiresAPIKey returns true if this provider needs a non-empty API key
// to function. Providers with their own auth mechanism (ollama native,
// local ONNX) return false.
func (p *EffectiveProvider) RequiresAPIKey() bool {
	if p == nil {
		return false
	}
	switch p.Type {
	case types.ProviderOllama, types.ProviderLocal:
		return false
	default:
		return p.APIKey == ""
	}
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
