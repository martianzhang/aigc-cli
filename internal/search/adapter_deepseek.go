package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
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
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

// SearchRaw also returns the raw Search API response for debugging.
func (p *DeepSeekProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	var dsResp struct {
		Results []struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"results"`
	}
	body, err := mergePayload(map[string]any{"query": query, "count": limit}, opts)
	if err != nil {
		return nil, nil, err
	}
	raw, err := p.post("/v1/tools/search", body, &dsResp)
	if err != nil {
		return nil, nil, err
	}

	results := make([]Result, 0, len(dsResp.Results))
	for _, r := range dsResp.Results {
		results = append(results, Result{URL: r.URL, Title: r.Title})
	}
	return results, raw, nil
}

// Preview renders the equivalent DeepSeek Search request with a masked key.
func (p *DeepSeekProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	body, err := mergePayload(map[string]any{"query": query, "count": limit}, opts)
	if err != nil {
		return nil, err
	}
	return &reqbuild.Plan{
		Method: http.MethodPost,
		URL:    p.baseURL + "/v1/tools/search",
		Body:   body,
		Headers: []reqbuild.Header{
			{Name: "Authorization", Value: "Bearer " + maskedKey(p.apiKey)},
		},
	}, nil
}

// PreviewFetch renders the equivalent DeepSeek Fetch request with a masked key.
func (p *DeepSeekProvider) PreviewFetch(url string) (*reqbuild.Plan, error) {
	body, err := json.Marshal(map[string]any{"url": url})
	if err != nil {
		return nil, fmt.Errorf("deepseek marshal: %w", err)
	}
	return &reqbuild.Plan{
		Method: http.MethodPost,
		URL:    p.baseURL + "/v1/tools/fetch",
		Body:   body,
		Headers: []reqbuild.Header{
			{Name: "Authorization", Value: "Bearer " + maskedKey(p.apiKey)},
		},
	}, nil
}

// Fetch retrieves a single page and returns its title (possibly empty) and
// Markdown content. Unlike Search it returns the whole document with no offset,
// so callers paginate the returned text themselves.
func (p *DeepSeekProvider) Fetch(url string) (title, content string, err error) {
	var dsResp struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	body, err := json.Marshal(map[string]any{"url": url})
	if err != nil {
		return "", "", fmt.Errorf("deepseek marshal: %w", err)
	}
	if _, err := p.post("/v1/tools/fetch", body, &dsResp); err != nil {
		return "", "", err
	}
	return dsResp.Title, dsResp.Content, nil
}

// post sends a JSON request to a DeepSeek tool endpoint, decodes the response
// body into out, and returns the raw body for debugging.
func (p *DeepSeekProvider) post(path string, bodyJSON json.RawMessage, out any) (json.RawMessage, error) {
	req, err := http.NewRequest("POST", p.baseURL+path, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("deepseek API error %d", resp.StatusCode)
	}
	raw, err := readRawBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("deepseek read: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("deepseek parse: %w", err)
	}
	return raw, nil
}
