package service

import "github.com/martianzhang/aigc-cli/internal/types"

// MaskConfigSecrets returns a deep copy of cfg with every secret value masked,
// safe to marshal or print. The input cfg is never modified.
func MaskConfigSecrets(cfg *types.Config) *types.Config {
	if cfg == nil {
		return nil
	}
	out := *cfg
	out.APIKey = maskIfSet(cfg.APIKey)
	out.BaseURL = RedactURLSecrets(cfg.BaseURL)
	out.HTTPProxy = RedactURLSecrets(cfg.HTTPProxy)
	out.Providers = maskNamedProviders(cfg.Providers)
	out.WebSearch = maskWebSearchProviders(cfg.WebSearch)
	return &out
}

// maskIfSet masks a secret only when present, so unset fields stay omitted.
func maskIfSet(secret string) string {
	if secret == "" {
		return ""
	}
	return MaskKey(secret)
}

// maskNamedProviders copies and masks every named provider definition.
func maskNamedProviders(in map[string]*types.NamedProvider) map[string]*types.NamedProvider {
	if in == nil {
		return nil
	}
	out := make(map[string]*types.NamedProvider, len(in))
	for name, p := range in {
		if p == nil {
			out[name] = nil
			continue
		}
		cp := *p
		cp.APIKey = maskIfSet(p.APIKey)
		cp.BaseURL = RedactURLSecrets(p.BaseURL)
		cp.HTTPProxy = RedactURLSecrets(p.HTTPProxy)
		out[name] = &cp
	}
	return out
}

// maskWebSearchProviders copies and masks every web search provider definition.
func maskWebSearchProviders(in map[string]*types.WebSearchProvider) map[string]*types.WebSearchProvider {
	if in == nil {
		return nil
	}
	out := make(map[string]*types.WebSearchProvider, len(in))
	for name, p := range in {
		if p == nil {
			out[name] = nil
			continue
		}
		cp := *p
		cp.APIKey = maskIfSet(p.APIKey)
		out[name] = &cp
	}
	return out
}
