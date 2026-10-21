package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
)

// DDGProvider wraps the DDG HTML search as a search Provider.
type DDGProvider struct{}

func NewDDGProvider() *DDGProvider { return &DDGProvider{} }

func (p *DDGProvider) Name() string { return "duckduckgo" }

func (p *DDGProvider) Search(query string, limit int) ([]Result, error) {
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

// SearchRaw returns the scraped URLs as a synthetic JSON document, since the
// DuckDuckGo provider has no vendor JSON response to preserve.
func (p *DDGProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	if len(opts.Extra) > 0 {
		return nil, nil, fmt.Errorf("duckduckgo does not support --inject-json")
	}
	urls, err := knowledge.DDGSearchURLs(query)
	if err != nil {
		return nil, nil, err
	}
	if len(urls) > limit {
		urls = urls[:limit]
	}
	results := make([]Result, len(urls))
	for i, u := range urls {
		results[i] = Result{URL: u, Title: u}
	}

	raw, err := json.Marshal(map[string]any{"urls": urls})
	if err != nil {
		return nil, nil, err
	}
	return results, raw, nil
}

// Preview renders the DuckDuckGo HTML search request. The CLI parses the HTML
// response locally; the curl only shows the request side.
func (p *DDGProvider) Preview(query string, _ int, opts Options) (*reqbuild.Plan, error) {
	if len(opts.Extra) > 0 {
		return nil, fmt.Errorf("duckduckgo does not support --inject-json")
	}
	return &reqbuild.Plan{
		Method: http.MethodGet,
		URL:    fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query)),
		Headers: []reqbuild.Header{
			{Name: "User-Agent", Value: "Mozilla/5.0 (compatible; aigc-cli/1.0)"},
		},
	}, nil
}
