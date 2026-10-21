package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
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
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

func (p *FirecrawlProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	payload := map[string]any{
		"query": query,
		"limit": limit,
	}
	bodyJSON, err := mergePayload(payload, opts)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest("POST", "https://api.firecrawl.dev/v1/search",
		bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("firecrawl: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("firecrawl API error %d", resp.StatusCode)
	}

	raw, err := readRawBody(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("firecrawl read: %w", err)
	}

	var fcResp struct {
		Success bool `json:"success"`
		Data    []struct {
			URL         string `json:"url"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &fcResp); err != nil {
		return nil, nil, fmt.Errorf("firecrawl parse: %w", err)
	}
	results := make([]Result, 0, len(fcResp.Data))
	for _, r := range fcResp.Data {
		results = append(results, Result{URL: r.URL, Title: r.Title})
	}
	return results, raw, nil
}

// Preview renders the equivalent Firecrawl Search request with a masked key.
func (p *FirecrawlProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	body, err := mergePayload(map[string]any{"query": query, "limit": limit}, opts)
	if err != nil {
		return nil, err
	}
	return &reqbuild.Plan{
		Method: http.MethodPost,
		URL:    "https://api.firecrawl.dev/v1/search",
		Body:   body,
		Headers: []reqbuild.Header{
			{Name: "Authorization", Value: "Bearer " + maskedKey(p.apiKey)},
		},
	}, nil
}
