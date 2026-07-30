// Package search provides a pluggable web search router with
// automatic provider fallback, quota tracking, and strategy-based selection.
package search

import (
	"fmt"
	"math/rand"
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
	Type   string
	APIKey string
	Tags   []string
	Quota  int
	Period string
	Weight int
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

// GetProvider returns the provider with the given name.
func (r *Router) GetProvider(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// SearchResult holds search results and the provider used.
type SearchResult struct {
	Results  []Result
	Provider string
}

// Search performs a search using the configured strategy.
func (r *Router) Search(query string, limit int, strategy string, preferred []string) (*SearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.providers) == 0 {
		return nil, fmt.Errorf("no search providers registered")
	}

	candidates := r.selectProviders(strategy, preferred)
	candidates = r.filterAvailable(candidates)

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no providers available (all exhausted quota or none configured)")
	}

	ordered := r.sortByWeight(candidates)

	var lastErr error
	for _, name := range ordered {
		p, ok := r.providers[name]
		if !ok {
			continue
		}
		results, err := p.Search(query, limit)
		if err != nil {
			lastErr = err
			continue
		}
		r.store.RecordUse(name)
		return &SearchResult{Results: results, Provider: name}, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
	}
	return nil, fmt.Errorf("no providers available")
}

func (r *Router) filterAvailable(candidates []string) []string {
	var result []string
	for _, name := range candidates {
		if r.store.CanUse(name) {
			result = append(result, name)
		}
	}
	return result
}

func (r *Router) sortByWeight(candidates []string) []string {
	type weighted struct {
		name   string
		weight int
	}
	var items []weighted
	for _, name := range candidates {
		w := 1
		if cfg, ok := r.configs[name]; ok && cfg.Weight > 0 {
			w = cfg.Weight
		}
		items = append(items, weighted{name: name, weight: w})
	}

	if len(items) <= 1 {
		return candidates
	}

	var result []string
	remaining := items
	for len(remaining) > 0 {
		totalWeight := 0
		for _, item := range remaining {
			totalWeight += item.weight
		}
		r := rand.Intn(totalWeight)
		cumulative := 0
		for i, item := range remaining {
			cumulative += item.weight
			if r < cumulative {
				result = append(result, item.name)
				remaining = append(remaining[:i], remaining[i+1:]...)
				break
			}
		}
	}
	return result
}

// selectProviders returns provider names in priority order.
func (r *Router) selectProviders(strategy string, preferred []string) []string {
	switch strings.ToLower(strategy) {
	case "manual":
		return preferred
	case "quality":
		result := r.filterByTag("quality")
		if len(result) == 0 {
			return r.allProviders()
		}
		return result
	case "cheap":
		result := r.filterByTag("free")
		if len(result) == 0 {
			return r.allProviders()
		}
		return result
	default: // "auto"
		free := r.filterByTag("free")
		quality := r.filterByTag("quality")

		if len(free) == 0 && len(quality) == 0 {
			return r.allProviders()
		}

		freeSet := make(map[string]bool)
		for _, f := range free {
			freeSet[f] = true
		}
		var nonFreeQuality []string
		for _, p := range quality {
			if !freeSet[p] {
				nonFreeQuality = append(nonFreeQuality, p)
			}
		}
		sort.Slice(free, func(i, j int) bool {
			return r.quotaRemaining(free[i]) < r.quotaRemaining(free[j])
		})
		return append(free, nonFreeQuality...)
	}
}

func (r *Router) allProviders() []string {
	var all []string
	for name := range r.providers {
		all = append(all, name)
	}
	return all
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
