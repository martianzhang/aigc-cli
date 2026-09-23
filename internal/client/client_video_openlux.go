package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// OpenLuxVideoBody builds the openlux video request body. A verbatim --json body
// wins; otherwise the typed fields are mapped. Shared by the real request and
// --dry-run/--verbose.
func OpenLuxVideoBody(req *types.VideoGenerateRequest) interface{} {
	bodyMap := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
	}
	if req.Size != "" {
		bodyMap["aspect_ratio"] = req.Size
	}
	if len(req.ImageURLs) > 0 {
		bodyMap["images"] = req.ImageURLs
	} else if len(req.ImageWithRoles) > 0 {
		images := make([]string, len(req.ImageWithRoles))
		for i, r := range req.ImageWithRoles {
			images[i] = r.URL
		}
		bodyMap["images"] = images
	}
	return req.BodyOrRaw(bodyMap)
}

// OpenLuxVideoSubmit sends a video generation request to api.openlux.ai's POST /v1/video/create.
func (c *Client) OpenLuxVideoSubmit(req *types.VideoGenerateRequest) (*types.OpenLuxVideoCreateResponse, error) {
	body, err := json.Marshal(OpenLuxVideoBody(req))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+OpenLuxVideoSubPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openlux video submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openlux video API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result types.OpenLuxVideoCreateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

// OpenLuxVideoQuery polls api.openlux.ai's video task status via GET /v1/video/query?id={id}.
func (c *Client) OpenLuxVideoQuery(taskID string) (*types.OpenLuxVideoQueryResponse, error) {
	path := openluxVideoQryPath + "?id=" + url.QueryEscape(taskID)
	var result types.OpenLuxVideoQueryResponse
	if err := c.doGet(path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
