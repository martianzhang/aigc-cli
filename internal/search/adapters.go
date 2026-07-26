package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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

// FirecrawlProvider wraps the Firecrawl search.
type FirecrawlProvider struct {
	apiKey string
}

func NewFirecrawlProvider(apiKey string) *FirecrawlProvider {
	return &FirecrawlProvider{apiKey: apiKey}
}

func (p *FirecrawlProvider) Name() string { return "firecrawl" }

func (p *FirecrawlProvider) Search(query string, limit int) ([]Result, error) {
	payload := map[string]interface{}{
		"query": query,
		"limit": limit,
	}
	bodyJSON, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.firecrawl.dev/v1/search",
		bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firecrawl: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("firecrawl API error %d", resp.StatusCode)
	}

	var fcResp struct {
		Success bool `json:"success"`
		Data    []struct {
			URL         string `json:"url"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fcResp); err != nil {
		return nil, fmt.Errorf("firecrawl parse: %w", err)
	}
	results := make([]Result, 0, len(fcResp.Data))
	for _, r := range fcResp.Data {
		results = append(results, Result{URL: r.URL, Title: r.Title})
	}
	return results, nil
}

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
