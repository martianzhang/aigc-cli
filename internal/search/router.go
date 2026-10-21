// Package search provides a small registry of pluggable web search providers.
// Selection is explicit: callers name one provider, and a failure is returned
// directly (no automatic rotation, weighted picking or quota accounting).
package search

import (
	"encoding/json"
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

// RawProvider is an optional extension for providers that can return the
// vendor's raw response body alongside the normalized results. It exists for
// debugging (`search` prints the raw body by default) without widening the core
// Provider interface or forcing every test double to implement it.
type RawProvider interface {
	Provider
	SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error)
}

// Result holds a single search result.
type Result struct {
	URL   string
	Title string
}

// Router holds the registered providers.
type Router struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRouter creates an empty provider registry.
func NewRouter() *Router {
	return &Router{providers: make(map[string]Provider)}
}

// Register adds a search provider to the router.
func (r *Router) Register(name string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
}

// GetProvider returns the provider with the given name.
func (r *Router) GetProvider(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// Names returns the names of all registered providers, sorted.
func (r *Router) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.namesLocked()
}

func (r *Router) namesLocked() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SearchResult holds search results and the provider used.
type SearchResult struct {
	Results  []Result
	Provider string
	// Raw is the provider's raw response body when it implements RawProvider.
	// Nil for providers without a raw form (and for test doubles).
	Raw json.RawMessage
}

// Search calls exactly one named provider. Unknown names and provider errors
// are returned to the caller; there is no fallback to another vendor.
func (r *Router) Search(query string, limit int, provider string, opts Options) (*SearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[provider]
	if !ok {
		available := strings.Join(r.namesLocked(), ", ")
		if available == "" {
			return nil, fmt.Errorf("provider %q is not configured", provider)
		}
		return nil, fmt.Errorf("provider %q is not configured (available: %s)", provider, available)
	}

	var results []Result
	var raw json.RawMessage
	var err error
	if rp, rawOK := p.(RawProvider); rawOK {
		results, raw, err = rp.SearchRaw(query, limit, opts)
	} else {
		if len(opts.Extra) > 0 {
			return nil, fmt.Errorf("provider %q does not support --inject-json", provider)
		}
		results, err = p.Search(query, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("search via %s: %w", provider, err)
	}
	return &SearchResult{Results: results, Provider: provider, Raw: raw}, nil
}
