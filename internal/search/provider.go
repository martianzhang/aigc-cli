package search

import (
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// NewProviderFromConfig creates a search provider from config.
func NewProviderFromConfig(cfg *types.WebSearchProvider) (Provider, error) {
	switch cfg.Type {
	case "duckduckgo":
		return NewDDGProvider(), nil
	case "firecrawl":
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("FIRECRAWL_API_KEY")
		}
		if apiKey == "" {
			return nil, fmt.Errorf("firecrawl requires api_key")
		}
		return NewFirecrawlProvider(apiKey), nil
	case "brave":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("brave requires api_key")
		}
		return NewBraveProvider(cfg.APIKey), nil
	case "doubao":
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("doubao requires api_key")
		}
		return NewDoubaoProvider(cfg.APIKey), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Type)
	}
}
