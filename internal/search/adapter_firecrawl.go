package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

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
