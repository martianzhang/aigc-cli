// Package search provides a pluggable web search router with
// automatic provider fallback, quota tracking, and strategy-based selection.
package search

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Provider defines a web search provider.
type Provider interface {
	// Name returns the provider identifier (e.g. "duckduckgo", "firecrawl").
	Name() string
	// Search returns up to limit result URLs for the given query.
	Search(query string, limit int) ([]Result, error)
}

// Result holds a single search result.
type Result struct {
	URL   string
	Title string
}

// ProviderInfo holds configuration for a registered provider.
type ProviderInfo struct {
	Type   string   // "duckduckgo", "firecrawl", "brave", etc.
	APIKey string   // optional API key
	Tags   []string // "free", "quality", etc.
	Quota  int      // max requests per period
	Period string   // "hourly", "daily", "monthly"
}

// Router manages multiple search providers and implements fallback.
type Router struct {
	mu        sync.RWMutex
	providers map[string]Provider
	configs   map[string]*ProviderInfo
	store     QuotaStore // SQLite-backed quota tracking
}

// QuotaStore tracks usage per provider.
type QuotaStore interface {
	CanUse(provider string) bool
	RecordUse(provider string)
	ResetIfNeeded(provider string)
	Usage(provider string) (used, total int)
}

// NewRouter creates a search router with the given providers and configs.
func NewRouter(store QuotaStore) *Router {
	return &Router{
		providers: make(map[string]Provider),
		configs:   make(map[string]*ProviderInfo),
		store:     store,
	}
}

// Register adds a search provider to the router.
func (r *Router) Register(name string, p Provider, info *ProviderInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
	if info != nil {
		r.configs[name] = info
	}
}

// Search performs a search using the configured strategy.
// Strategies: "auto" (free providers first), "quality" (quality tagged only),
// "cheap" (free only, no paid fallback), "manual" (specific provider list).
func (r *Router) Search(query string, limit int, strategy string, preferred []string) ([]Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.providers) == 0 {
		return nil, fmt.Errorf("no search providers registered")
	}

	// Select providers based on strategy
	candidates := r.selectProviders(strategy, preferred)

	var lastErr error
	for _, name := range candidates {
		p, ok := r.providers[name]
		if !ok {
			continue
		}
		// Check quota
		if !r.store.CanUse(name) {
			continue
		}
		results, err := p.Search(query, limit)
		if err != nil {
			lastErr = err
			continue
		}
		r.store.RecordUse(name)
		return results, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
	}
	return nil, fmt.Errorf("no providers available (all exhausted quota or none configured)")
}

// selectProviders returns provider names in priority order.
func (r *Router) selectProviders(strategy string, preferred []string) []string {
	switch strings.ToLower(strategy) {
	case "manual":
		return preferred
	case "quality":
		return r.filterByTag("quality")
	case "cheap":
		return r.filterByTag("free")
	default: // "auto"
		// Free providers first, sorted by remaining quota
		free := r.filterByTag("free")
		paid := r.filterByTag("quality")
		// Remove free from paid
		freeSet := make(map[string]bool)
		for _, f := range free {
			freeSet[f] = true
		}
		var nonFreePaid []string
		for _, p := range paid {
			if !freeSet[p] {
				nonFreePaid = append(nonFreePaid, p)
			}
		}
		sort.Slice(free, func(i, j int) bool {
			return r.quotaRemaining(free[i]) < r.quotaRemaining(free[j])
		})
		return append(free, nonFreePaid...)
	}
}

func (r *Router) filterByTag(tag string) []string {
	var result []string
	for name, info := range r.configs {
		for _, t := range info.Tags {
			if t == tag {
				result = append(result, name)
				break
			}
		}
	}
	// If no configs, include all providers as fallback
	if len(result) == 0 {
		for name := range r.providers {
			result = append(result, name)
		}
	}
	return result
}

func (r *Router) quotaRemaining(name string) int {
	used, total := r.store.Usage(name)
	if total <= 0 {
		return 999 // unlimited
	}
	return total - used
}

// Close releases router resources.
func (r *Router) Close() {}
