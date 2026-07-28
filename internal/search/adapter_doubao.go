package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
	payload := map[string]interface{}{
		"Query":      query,
		"SearchType": "web",
		"Count":      limit,
	}
	bodyJSON, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "https://open.feedcoopapi.com/search_api/web_search",
		bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doubao search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("doubao API error %d", resp.StatusCode)
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
	if err := json.NewDecoder(resp.Body).Decode(&dbResp); err != nil {
		return nil, fmt.Errorf("doubao parse: %w", err)
	}

	if dbResp.ResponseMetadata != nil && dbResp.ResponseMetadata.Error != nil {
		return nil, fmt.Errorf("doubao API error %s: %s",
			dbResp.ResponseMetadata.Error.Code,
			dbResp.ResponseMetadata.Error.Message)
	}

	results := dbResp.Result
	if results == nil || len(results.WebResults) == 0 {
		return nil, nil
	}

	out := make([]Result, 0, len(results.WebResults))
	for _, d := range results.WebResults {
		out = append(out, Result{URL: d.URL, Title: d.Title})
	}
	return out, nil
}
