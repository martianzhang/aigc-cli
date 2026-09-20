package mcp

import (
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

// providerArgDesc is the shared schema description for the optional per-call
// provider argument. Providers can only be referenced by name from
// config.providers — never by raw URL or path.
const providerArgDesc = "Name of a provider from config.providers (whitelist). A URL is not accepted."

// resolveProviderRef resolves the optional per-call `provider` argument for a
// command against the configured provider whitelist.
//
// An empty ref selects the command's default provider (cfg.cmdProvider),
// identical to a call that never passed the argument.
//
// A non-empty ref is accepted only when it is a bare provider name that exists
// in config.providers. URL- and path-shaped values are rejected before the
// whitelist lookup, so a raw base URL can never be turned into a provider.
//
// The second return value is a non-empty, user-facing message when the ref is
// rejected; it is empty on success.
func (cfg *Config) resolveProviderRef(cmdName, ref string) (*provider.EffectiveProvider, string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return cfg.cmdProvider(cmdName), ""
	}
	if msg := rejectProviderShape(ref); msg != "" {
		return nil, msg
	}
	if err := provider.ValidateProviderRef(ref, cfg.Providers); err != nil {
		return nil, err.Error()
	}
	global := &provider.GlobalConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Proxy:   cfg.Proxy,
	}
	return provider.ResolveCmdProvider(nil, ref, cfg.Providers, global), ""
}

// rejectProviderShape rejects provider refs that look like a URL or a file
// path. Only bare provider names from config.providers are accepted.
func rejectProviderShape(ref string) string {
	if strings.Contains(ref, "://") || strings.ContainsAny(ref, `/\@`) {
		return fmt.Sprintf(
			"provider must be the name of a provider from config.providers; URL or path values are not accepted (got %q)",
			ref,
		)
	}
	return ""
}
