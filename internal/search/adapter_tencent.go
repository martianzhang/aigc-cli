package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
)

// defaultTencentWSAURL is the Tencent Yuanbao Search (WSA) API root. The API
// KEY compatible mode exposes POST /SearchPro with a Bearer token.
const defaultTencentWSAURL = "https://api.wsa.cloud.tencent.com"

// TencentWSAProvider wraps Tencent Yuanbao Search (元宝搜索, WSA) in API KEY
// mode. Its Pages field is an array of JSON strings, which is decoded into the
// router's Result shape.
type TencentWSAProvider struct {
	apiKey string
	// baseURL is the API root, overridable in tests; it defaults to
	// defaultTencentWSAURL.
	baseURL string
}

func NewTencentWSAProvider(apiKey string) *TencentWSAProvider {
	return &TencentWSAProvider{apiKey: apiKey, baseURL: defaultTencentWSAURL}
}

func (p *TencentWSAProvider) Name() string { return "wsa" }

// Search queries the SearchPro API and maps its results onto the router's
// Result shape (title + URL).
func (p *TencentWSAProvider) Search(query string, limit int) ([]Result, error) {
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

// SearchRaw also returns the raw SearchPro response for debugging.
func (p *TencentWSAProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	body, err := mergePayload(map[string]any{
		"Query": query,
		"Cnt":   tencentResultCount(limit),
	}, opts)
	if err != nil {
		return nil, nil, err
	}
	raw, pages, err := p.post(body)
	if err != nil {
		return nil, nil, err
	}

	results := make([]Result, 0, len(pages))
	for _, page := range pages {
		var doc struct {
			URL   string `json:"url"`
			Title string `json:"title"`
		}
		if page == "" {
			continue
		}
		if err := json.Unmarshal([]byte(page), &doc); err != nil {
			return nil, nil, fmt.Errorf("wsa parse page: %w", err)
		}
		results = append(results, Result{URL: doc.URL, Title: doc.Title})
	}
	// Cnt only accepts 10/20/30/40/50, so the API may return more entries than
	// the caller asked for. Trim to keep `-n` and the chat/MCP limit exact.
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, raw, nil
}

// Preview renders the equivalent SearchPro request with a masked key.
func (p *TencentWSAProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	body, err := mergePayload(map[string]any{
		"Query": query,
		"Cnt":   tencentResultCount(limit),
	}, opts)
	if err != nil {
		return nil, err
	}
	return &reqbuild.Plan{
		Method: http.MethodPost,
		URL:    p.baseURL + "/SearchPro",
		Body:   body,
		Headers: []reqbuild.Header{
			{Name: "Authorization", Value: "Bearer " + maskedKey(p.apiKey)},
		},
	}, nil
}

// tencentResultCount maps a caller limit onto a Cnt value the API accepts
// (10/20/30/40/50). The CLI caps its own limit at 20, but this stays correct
// for direct callers.
func tencentResultCount(limit int) int {
	if limit <= 10 {
		return 10
	}
	cnt := ((limit + 9) / 10) * 10
	if cnt > 50 {
		return 50
	}
	return cnt
}

// post sends a SearchPro request and returns the raw body plus the decoded
// Pages entries. Tencent's error envelope carries Error.Code/Message, which is
// included in the returned error so auth/quota failures are diagnosable.
func (p *TencentWSAProvider) post(bodyJSON json.RawMessage) (json.RawMessage, []string, error) {
	req, err := http.NewRequest(http.MethodPost, p.baseURL+"/SearchPro", bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("wsa request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := readRawBody(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("wsa read: %w", err)
	}

	var envelope struct {
		Response struct {
			Pages     []string `json:"Pages"`
			RequestID string   `json:"RequestId"`
			Error     *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		if resp.StatusCode >= 400 {
			return nil, nil, fmt.Errorf("wsa API error %d", resp.StatusCode)
		}
		return nil, nil, fmt.Errorf("wsa parse: %w", err)
	}
	if e := envelope.Response.Error; e != nil {
		return nil, nil, fmt.Errorf("wsa API error %s: %s", e.Code, e.Message)
	}
	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("wsa API error %d", resp.StatusCode)
	}
	return raw, envelope.Response.Pages, nil
}
