package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// defaultDeepSeekBaseURL is the DeepSeek API root. Both the in-house Search and
// Fetch endpoints hang off it. Calls are billed per successful request
// ($0.75 / 1000), independent of the number of results or the response size.
const defaultDeepSeekBaseURL = "https://api.deepseek.com"

// DeepSeekProvider wraps DeepSeek's in-house Search (POST /v1/tools/search) and
// Fetch (POST /v1/tools/fetch) APIs. It therefore satisfies both the Provider
// and Fetcher interfaces.
type DeepSeekProvider struct {
	apiKey string
	// baseURL is the API root, overridable in tests; it defaults to
	// defaultDeepSeekBaseURL.
	baseURL string
}

func NewDeepSeekProvider(apiKey string) *DeepSeekProvider {
	return &DeepSeekProvider{apiKey: apiKey, baseURL: defaultDeepSeekBaseURL}
}

func (p *DeepSeekProvider) Name() string { return "deepseek" }

// Search queries the Search API and maps the structured results onto the
// router's Result shape (title + URL). An empty result set is a normal, free
// response.
func (p *DeepSeekProvider) Search(query string, limit int) ([]Result, error) {
	var dsResp struct {
		Results []struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"results"`
	}
	if err := p.post("/v1/tools/search", map[string]any{"query": query, "count": limit}, &dsResp); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(dsResp.Results))
	for _, r := range dsResp.Results {
		results = append(results, Result{URL: r.URL, Title: r.Title})
	}
	return results, nil
}

// Fetch retrieves a single page and returns its title (possibly empty) and
// Markdown content. Unlike Search it returns the whole document with no offset,
// so callers paginate the returned text themselves.
func (p *DeepSeekProvider) Fetch(url string) (title, content string, err error) {
	var dsResp struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := p.post("/v1/tools/fetch", map[string]any{"url": url}, &dsResp); err != nil {
		return "", "", err
	}
	return dsResp.Title, dsResp.Content, nil
}

// post sends a JSON request to a DeepSeek tool endpoint and decodes the response
// body into out.
func (p *DeepSeekProvider) post(path string, payload map[string]any, out any) error {
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("deepseek marshal: %w", err)
	}

	req, err := http.NewRequest("POST", p.baseURL+path, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("deepseek request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("deepseek API error %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("deepseek parse: %w", err)
	}
	return nil
}
