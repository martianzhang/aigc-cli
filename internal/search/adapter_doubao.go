package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
)

// DoubaoProvider wraps the Volcengine Doubao (豆包) Search API (Custom版).
type DoubaoProvider struct {
	apiKey string
}

func NewDoubaoProvider(apiKey string) *DoubaoProvider {
	return &DoubaoProvider{apiKey: apiKey}
}

func (p *DoubaoProvider) Name() string { return "doubao" }

func (p *DoubaoProvider) Search(query string, limit int) ([]Result, error) {
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

func (p *DoubaoProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	payload := map[string]any{
		"Query":      query,
		"SearchType": "web",
		"Count":      limit,
	}
	bodyJSON, err := mergePayload(payload, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest("POST", "https://open.feedcoopapi.com/search_api/web_search",
		bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("doubao search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("doubao API error %d", resp.StatusCode)
	}

	raw, err := readRawBody(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("doubao read: %w", err)
	}

	var dbResp struct {
		ResponseMetadata *struct {
			RequestID string `json:"RequestId"`
			Error     *struct {
				CodeN   int    `json:"CodeN"`
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
		Result *struct {
			ResultCount int `json:"ResultCount"`
			WebResults  []struct {
				URL   string `json:"Url"`
				Title string `json:"Title"`
			} `json:"WebResults"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(raw, &dbResp); err != nil {
		return nil, nil, fmt.Errorf("doubao parse: %w", err)
	}

	if dbResp.ResponseMetadata != nil && dbResp.ResponseMetadata.Error != nil {
		return nil, nil, fmt.Errorf("doubao API error %s: %s",
			dbResp.ResponseMetadata.Error.Code,
			dbResp.ResponseMetadata.Error.Message)
	}

	results := dbResp.Result
	if results == nil || len(results.WebResults) == 0 {
		return nil, raw, nil
	}

	out := make([]Result, 0, len(results.WebResults))
	for _, d := range results.WebResults {
		out = append(out, Result{URL: d.URL, Title: d.Title})
	}
	return out, raw, nil
}

// Preview renders the equivalent Doubao Search request with a masked key.
func (p *DoubaoProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	body, err := mergePayload(map[string]any{
		"Query":      query,
		"SearchType": "web",
		"Count":      limit,
	}, opts)
	if err != nil {
		return nil, err
	}
	return &reqbuild.Plan{
		Method: http.MethodPost,
		URL:    "https://open.feedcoopapi.com/search_api/web_search",
		Body:   body,
		Headers: []reqbuild.Header{
			{Name: "Authorization", Value: "Bearer " + maskedKey(p.apiKey)},
		},
	}, nil
}
