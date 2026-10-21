package search

import (
	"sort"
	"strings"
)

// Fetcher is the subset of providers that can retrieve and format a single web
// page (e.g. DeepSeek's POST /v1/tools/fetch, which returns Markdown directly).
// Not every search provider offers this: the built-in DuckDuckGo scraper and
// Brave are search-only, so callers must type-assert rather than assume.
type Fetcher interface {
	// Name matches the provider's search name.
	Name() string
	// Fetch returns the page title (possibly empty) and its content as Markdown.
	Fetch(url string) (title, content string, err error)
}

// ResolveFetcher returns the named provider when it implements Fetcher. The
// boolean is false when the provider is unknown or search-only.
func ResolveFetcher(r *Router, name string) (Fetcher, bool) {
	p, ok := r.GetProvider(strings.TrimSpace(name))
	if !ok {
		return nil, false
	}
	f, ok := p.(Fetcher)
	return f, ok
}

// FetcherNames returns the registered providers that support fetch, sorted.
func (r *Router) FetcherNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name, p := range r.providers {
		if _, ok := p.(Fetcher); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
