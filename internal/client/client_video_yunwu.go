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

// YunwuVideoSubmit sends a video generation request to yunwu.ai's POST /v1/video/create.
func (c *Client) YunwuVideoSubmit(req *types.VideoGenerateRequest) (*types.YunwuVideoCreateResponse, error) {
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

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+yunwuVideoSubPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("yunwu video submit failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yunwu video API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result types.YunwuVideoCreateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

// YunwuVideoQuery polls yunwu.ai's video task status via GET /v1/video/query?id={id}.
func (c *Client) YunwuVideoQuery(taskID string) (*types.YunwuVideoQueryResponse, error) {
	path := yunwuVideoQryPath + "?id=" + url.QueryEscape(taskID)
	var result types.YunwuVideoQueryResponse
	if err := c.doGet(path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
