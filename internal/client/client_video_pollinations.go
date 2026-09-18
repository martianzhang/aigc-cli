package client

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// PollinationsVideoURL builds the URL for pollinations.ai's synchronous video
// endpoint: GET <root>/video/{prompt}?model=...&duration=...
//
// The media endpoint lives at the API root (e.g. https://gen.pollinations.ai),
// not under the /vN version prefix used by the OpenAI-compatible routes, so the
// version segment is stripped from the client base URL before joining.
func (c *Client) PollinationsVideoURL(req *types.VideoGenerateRequest) string {
	root := strings.TrimRight(c.baseURL, "/")
	if HasVersionSuffix(root) {
		if i := strings.LastIndex(root, "/"); i > 0 {
			root = root[:i]
		}
	}

	q := url.Values{}
	if req.Model != "" {
		q.Set("model", req.Model)
	}
	if req.Duration != nil && *req.Duration > 0 {
		q.Set("duration", strconv.Itoa(*req.Duration))
	}
	if req.Seed != nil && *req.Seed != 0 {
		q.Set("seed", strconv.Itoa(*req.Seed))
	}
	if frame := pollinationsStartFrame(req); frame != "" {
		q.Set("image", frame)
	}

	rawURL := root + "/video/" + url.PathEscape(req.Prompt)
	if enc := q.Encode(); enc != "" {
		rawURL += "?" + enc
	}
	return rawURL
}

// pollinationsStartFrame returns the first supplied start-frame image, if any.
func pollinationsStartFrame(req *types.VideoGenerateRequest) string {
	for _, r := range req.ImageWithRoles {
		if r.Role == "first_frame" && r.URL != "" {
			return r.URL
		}
	}
	if len(req.ImageURLs) > 0 {
		return req.ImageURLs[0]
	}
	return ""
}

// PollinationsVideoGenerate generates a video via pollinations.ai's synchronous
// GET /video/{prompt} endpoint and returns the raw bytes plus the response
// content type. Generation can take a few minutes; pollinations finishes
// server-side even if the client disconnects, so re-issuing the identical
// request is safe.
func (c *Client) PollinationsVideoGenerate(req *types.VideoGenerateRequest) ([]byte, string, error) {
	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodGet, c.PollinationsVideoURL(req), nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	for k, v := range c.defaultHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isTimeoutError(err) {
			return nil, "", fmt.Errorf("API request timed out: %w\n%s", err, timeoutHint())
		}
		return nil, "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, resp.Header.Get("Content-Type"), nil
}
