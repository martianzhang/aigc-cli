package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
)

// defaultExaBaseURL is the Exa API root. Both the Search (/search) and Contents
// (/contents) endpoints hang off it.
const defaultExaBaseURL = "https://api.exa.ai"

// ExaProvider wraps Exa's neural search (POST /search) and page contents
// (POST /contents) APIs, so it satisfies both Provider and Fetcher.
type ExaProvider struct {
	apiKey string
	// baseURL is the API root, overridable in tests; it defaults to
	// defaultExaBaseURL.
	baseURL string
}

func NewExaProvider(apiKey string) *ExaProvider {
	return &ExaProvider{apiKey: apiKey, baseURL: defaultExaBaseURL}
}

func (p *ExaProvider) Name() string { return "exa" }

// Search queries the Search API and maps its results onto the router's Result
// shape (title + URL). An empty result set is a normal response.
func (p *ExaProvider) Search(query string, limit int) ([]Result, error) {
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

// SearchRaw also returns the raw Search API response for debugging.
func (p *ExaProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	var exaResp struct {
		Results []struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		} `json:"results"`
	}
	body, err := mergePayload(map[string]any{"query": query, "numResults": limit}, opts)
	if err != nil {
		return nil, nil, err
	}
	raw, err := p.post("/search", body, &exaResp)
	if err != nil {
		return nil, nil, err
	}

	results := make([]Result, 0, len(exaResp.Results))
	for _, r := range exaResp.Results {
		results = append(results, Result{URL: r.URL, Title: r.Title})
	}
	return results, raw, nil
}

// Preview renders the equivalent Exa Search request with a masked key.
func (p *ExaProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	body, err := mergePayload(map[string]any{"query": query, "numResults": limit}, opts)
	if err != nil {
		return nil, err
	}
	return &reqbuild.Plan{
		Method: http.MethodPost,
		URL:    p.baseURL + "/search",
		Body:   body,
		Headers: []reqbuild.Header{
			{Name: "x-api-key", Value: maskedKey(p.apiKey)},
		},
	}, nil
}

// Fetch retrieves a single page through the Contents API and returns its title
// (possibly empty) and full text. Callers paginate the returned text.
func (p *ExaProvider) Fetch(url string) (title, content string, err error) {
	var exaResp struct {
		Results []struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"results"`
	}
	payload := map[string]any{"urls": []string{url}, "text": true}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("exa marshal: %w", err)
	}
	if _, err := p.post("/contents", body, &exaResp); err != nil {
		return "", "", err
	}
	if len(exaResp.Results) == 0 {
		return "", "", fmt.Errorf("exa returned no contents for %s", url)
	}
	return exaResp.Results[0].Title, exaResp.Results[0].Text, nil
}

// post sends a JSON request to an Exa endpoint, decodes the response body into
// out, and returns the raw body for debugging. Exa's error envelope carries a
// human-readable `error` string, which is included in the returned error to
// make auth/quota failures diagnosable.
func (p *ExaProvider) post(path string, bodyJSON json.RawMessage, out any) (json.RawMessage, error) {
	req, err := http.NewRequest("POST", p.baseURL+path, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exa request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var envelope struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&envelope)
		if msg := strings.TrimSpace(envelope.Error); msg != "" {
			return nil, fmt.Errorf("exa API error %d: %s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("exa API error %d", resp.StatusCode)
	}
	raw, err := readRawBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("exa read: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("exa parse: %w", err)
	}
	return raw, nil
}
