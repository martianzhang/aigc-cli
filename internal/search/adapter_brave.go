package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	apiURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), limit)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("brave request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Subscription-Token", p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("brave API error %d", resp.StatusCode)
	}

	var braveResp struct {
		Web struct {
			Results []struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&braveResp); err != nil {
		return nil, fmt.Errorf("brave parse: %w", err)
	}

	results := make([]Result, 0, len(braveResp.Web.Results))
	for _, r := range braveResp.Web.Results {
		results = append(results, Result{URL: r.URL, Title: r.Title})
	}
	return results, nil
}
