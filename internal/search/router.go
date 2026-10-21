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
	mu            sync.RWMutex
	providers     map[string]Provider
	providerTypes map[string]string
	unavailable   map[string]unavailableProvider
}

type unavailableProvider struct {
	typeName string
	err      error
}

// ProviderStatus is a diagnostic view of one configured provider.
type ProviderStatus struct {
	Name      string
	Type      string
	Search    bool
	Fetch     bool
	Available bool
	Err       string
}

// NewRouter creates an empty provider registry.
func NewRouter() *Router {
	return &Router{
		providers:     make(map[string]Provider),
		providerTypes: make(map[string]string),
		unavailable:   make(map[string]unavailableProvider),
	}
}

// Register adds a search provider to the router.
func (r *Router) Register(name string, p Provider) {
	r.RegisterWithType(name, p, "")
}

// RegisterWithType adds a provider and records the configured type for
// --list-providers.
func (r *Router) RegisterWithType(name string, p Provider, typeName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if typeName == "" {
		typeName = p.Name()
	}
	r.providers[name] = p
	r.providerTypes[name] = typeName
	delete(r.unavailable, name)
}

// RegisterUnavailable records a configured provider that failed construction,
// so callers can explain why it is missing instead of saying "not configured".
func (r *Router) RegisterUnavailable(name, typeName string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.providers, name)
	r.providerTypes[name] = typeName
	r.unavailable[name] = unavailableProvider{typeName: typeName, err: err}
}

// Unavailable returns the construction error for a configured provider.
func (r *Router) Unavailable(name string) (error, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.unavailable[name]
	return u.err, ok
}

// Statuses returns every configured provider (available or not), sorted by
// name, for --list-providers and diagnostics.
func (r *Router) Statuses() []ProviderStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers)+len(r.unavailable))
	seen := make(map[string]bool, len(r.providers)+len(r.unavailable))
	for name := range r.providers {
		names = append(names, name)
		seen[name] = true
	}
	for name := range r.unavailable {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	out := make([]ProviderStatus, 0, len(names))
	for _, name := range names {
		status := ProviderStatus{Name: name, Type: r.providerTypes[name]}
		if p, ok := r.providers[name]; ok {
			status.Available = true
			status.Search = true
			_, status.Fetch = p.(Fetcher)
		} else {
			status.Err = r.unavailable[name].err.Error()
		}
		out = append(out, status)
	}
	return out
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
		if bad, exists := r.unavailable[provider]; exists {
			return nil, fmt.Errorf("provider %q is configured but unavailable: %w", provider, bad.err)
		}
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
