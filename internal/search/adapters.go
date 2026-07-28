package search

import (
	"github.com/martianzhang/aigc-cli/internal/knowledge"
)

// DDGProvider wraps the DDG HTML search as a search Provider.
type DDGProvider struct{}

func NewDDGProvider() *DDGProvider { return &DDGProvider{} }

func (p *DDGProvider) Name() string { return "duckduckgo" }

func (p *DDGProvider) Search(query string, limit int) ([]Result, error) {
	urls, err := knowledge.DDGSearchURLs(query)
	if err != nil {
		return nil, err
	}
	if len(urls) > limit {
		urls = urls[:limit]
	}
	results := make([]Result, len(urls))
	for i, u := range urls {
		results[i] = Result{URL: u, Title: u}
	}
	return results, nil
}
