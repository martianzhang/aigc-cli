package search

import (
	"slices"
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

// ResolveFetcher picks the Fetcher for a provider spec, mirroring the
// vocabulary of ResolveStrategy:
//
//	<name>       -> that provider, when it implements Fetcher
//	"" | "auto"  -> the first fetch-capable provider in name order
//	"free"|"cheap" -> first fetch-capable provider tagged "free"
//	"quality"    -> first fetch-capable provider tagged "quality"
//
// The boolean is false when no fetch-capable provider matches.
func ResolveFetcher(r *Router, spec string) (Fetcher, bool) {
	name := strings.TrimSpace(spec)
	switch strings.ToLower(name) {
	case "", "auto":
		return r.firstFetcher("")
	case "free", "cheap":
		return r.firstFetcher("free")
	case "quality":
		return r.firstFetcher("quality")
	default:
		p, ok := r.GetProvider(name)
		if !ok {
			return nil, false
		}
		f, ok := p.(Fetcher)
		return f, ok
	}
}

// firstFetcher returns the first fetch-capable provider in deterministic name
// order, optionally restricted to providers carrying tag.
func (r *Router) firstFetcher(tag string) (Fetcher, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		if tag != "" {
			info := r.configs[name]
			if info == nil || !slices.Contains(info.Tags, tag) {
				continue
			}
		}
		if f, ok := r.providers[name].(Fetcher); ok {
			return f, true
		}
	}
	return nil, false
}
