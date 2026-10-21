package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
)

// BraveProvider wraps the Brave Search API.
type BraveProvider struct {
	apiKey string
}

func NewBraveProvider(apiKey string) *BraveProvider {
	return &BraveProvider{apiKey: apiKey}
}

func (p *BraveProvider) Name() string { return "brave" }

func (p *BraveProvider) Search(query string, limit int) ([]Result, error) {
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

func (p *BraveProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	apiURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), limit)
	apiURL, err := injectQuery(apiURL, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("brave request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("brave search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("brave API error %d", resp.StatusCode)
	}

	raw, err := readRawBody(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("brave read: %w", err)
	}

	var braveResp struct {
		Web struct {
			Results []struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &braveResp); err != nil {
		return nil, nil, fmt.Errorf("brave parse: %w", err)
	}

	results := make([]Result, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, Result{URL: r.URL, Title: r.Title})
	}
	return results, raw, nil
}

// Preview renders the equivalent Brave Search request with a masked API key.
func (p *BraveProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	apiURL, err := injectQuery(fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), limit), opts)
	if err != nil {
		return nil, err
	}
	return &reqbuild.Plan{
		Method: http.MethodGet,
		URL:    apiURL,
		Headers: []reqbuild.Header{
			{Name: "Accept", Value: "application/json"},
			{Name: "Accept-Encoding", Value: "gzip"},
			{Name: "X-Subscription-Token", Value: maskedKey(p.apiKey)},
		},
	}, nil
}
