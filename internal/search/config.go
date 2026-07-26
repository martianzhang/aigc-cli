package search

import (
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ConfigFromTypes converts the types.WebSearch config into router configs.
func ConfigFromTypes(providers map[string]*types.WebSearchProvider) map[string]*ProviderInfo {
	if providers == nil {
		return nil
	}
	info := make(map[string]*ProviderInfo)
	for name, p := range providers {
		info[name] = &ProviderInfo{
			Type:   p.Type,
			APIKey: p.APIKey,
			Tags:   p.Tags,
			Quota:  p.Quota,
			Period: p.Period,
		}
	}
	return info
}

// DefaultConfigs returns default provider configs if none are configured.
// Currently defaults to duckduckgo (free, no key needed).
func DefaultConfigs() map[string]*ProviderInfo {
	return map[string]*ProviderInfo{
		"duckduckgo": {
			Type:   "duckduckgo",
			Tags:   []string{"free"},
			Quota:  0, // unlimited
			Period: "",
		},
	}
}
