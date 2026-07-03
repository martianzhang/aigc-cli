package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/martianzhang/apimart-cli/internal/types"
)

// ---------------------------------------------------------------------------
// OpenRouter Image — Dedicated Image API (POST /api/v1/images)
// This is the primary path for all image models on OpenRouter.
// Returns OpenAI-compatible response format with b64_json.
// ---------------------------------------------------------------------------

// Image generation and video generation on OpenRouter can take 60-120s.
const openrouterRequestTimeout = 120 * time.Second

// OpenRouterDedicatedImage sends a text-to-image request via OpenRouter's
// dedicated Image API (POST /v1/images). Returns standard OpenAI-compatible response.
// Supports input_references (image-to-image) via req.ImageURLs.
func (c *Client) OpenRouterDedicatedImage(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	// Build request body with OpenRouter-specific field mapping.
	// OpenRouter uses "input_references" instead of "image_urls" for reference images.
	bodyMap := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
	}
	if req.Size != "" {
		bodyMap["size"] = req.Size
	}
	if req.Quality != "" {
		bodyMap["quality"] = req.Quality
	}
	if req.OutputFormat != "" {
		bodyMap["output_format"] = req.OutputFormat
	}
	if req.Background != "" {
		bodyMap["background"] = req.Background
	}
	if req.Resolution != "" {
		bodyMap["resolution"] = req.Resolution
	}
	if req.N != nil {
		bodyMap["n"] = *req.N
	}
	if req.OutputCompression != nil {
		bodyMap["output_compression"] = *req.OutputCompression
	}

	// Map image_urls → input_references (OpenRouter format)
	if len(req.ImageURLs) > 0 {
		refs := make([]map[string]interface{}, len(req.ImageURLs))
		for i, u := range req.ImageURLs {
			refs[i] = map[string]interface{}{
				"type": "image_url",
				"image_url": map[string]string{
					"url": u,
				},
			}
		}
		bodyMap["input_references"] = refs
	}

	oldTimeout := c.httpClient.Timeout
	c.httpClient.Timeout = openrouterRequestTimeout
	defer func() { c.httpClient.Timeout = oldTimeout }()

	headers := openRouterHeaders()
	var result types.OpenAIImageResponse
	if err := c.doJSONWithHeaders(http.MethodPost, "/images", bodyMap, &result, headers); err != nil {
		return nil, err
	}
	return &result, nil
}

// ---------------------------------------------------------------------------
// OpenRouter Video — dedicated async API (POST /api/v1/videos)
// ---------------------------------------------------------------------------

// OpenRouterVideoSubmit submits a video generation job and returns the job info.
func (c *Client) OpenRouterVideoSubmit(req *types.OpenRouterVideoRequest) (*types.OpenRouterVideoSubmitResponse, error) {
	headers := openRouterHeaders()
	var result types.OpenRouterVideoSubmitResponse
	if err := c.doJSONWithHeaders(http.MethodPost, "/videos", req, &result, headers); err != nil {
		return nil, err
	}
	return &result, nil
}

// OpenRouterVideoPoll polls the video job status using the polling URL.
func (c *Client) OpenRouterVideoPoll(pollingURL string) (*types.OpenRouterVideoStatusResponse, error) {
	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodGet, pollingURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create poll request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	c.setOpenRouterHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("poll request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read poll response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("poll returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result types.OpenRouterVideoStatusResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse poll response: %w", err)
	}
	return &result, nil
}

// OpenRouterVideoGet queries a video job by its ID via GET /v1/videos/{id}.
func (c *Client) OpenRouterVideoGet(jobID string) (*types.OpenRouterVideoStatusResponse, error) {
	path := "/videos/" + jobID
	headers := openRouterHeaders()
	var result types.OpenRouterVideoStatusResponse
	if err := c.doGetWithHeaders(path, &result, headers); err != nil {
		return nil, err
	}
	return &result, nil
}

// OpenRouterVideoDownload downloads the video from an unsigned URL and saves it.
func (c *Client) OpenRouterVideoDownload(url, dest string) error {
	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download returned status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read download response: %w", err)
	}

	return os.WriteFile(dest, data, 0644)
}

// OpenRouterVideoPollUntilComplete polls a video job until completion or failure.
// pollInterval: time between polls (default 30s if zero).
// maxWait: maximum total wait time (default 5min if zero).
func (c *Client) OpenRouterVideoPollUntilComplete(pollingURL string, pollInterval, maxWait time.Duration) (*types.OpenRouterVideoStatusResponse, error) {
	if pollInterval == 0 {
		pollInterval = 30 * time.Second
	}
	if maxWait == 0 {
		maxWait = 5 * time.Minute
	}

	start := time.Now()
	for {
		// Check for cancellation
		select {
		case <-c.requestContext().Done():
			return nil, c.requestContext().Err()
		default:
		}

		if time.Since(start) > maxWait {
			return nil, fmt.Errorf("video polling timed out after %v\n  The job may still be running. Use: apimart-cli video --job-id %s", maxWait, extractJobID(pollingURL))
		}

		resp, err := c.OpenRouterVideoPoll(pollingURL)
		if err != nil {
			return nil, fmt.Errorf("poll failed: %w", err)
		}

		switch resp.Status {
		case "completed":
			return resp, nil
		case "failed", "cancelled", "expired":
			errMsg := resp.Error
			if errMsg == "" {
				errMsg = resp.Status
			}
			return nil, fmt.Errorf("video generation %s: %s", resp.Status, errMsg)
		default:
			// pending / running — keep waiting
			select {
			case <-time.After(pollInterval):
			case <-c.requestContext().Done():
				return nil, c.requestContext().Err()
			}
		}
	}
}

// extractJobID extracts the job ID from a polling URL (e.g. /v1/videos/job_xxx/poll).
func extractJobID(pollingURL string) string {
	parts := strings.Split(strings.TrimRight(pollingURL, "/"), "/")
	for i, p := range parts {
		if p == "videos" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return pollingURL // fallback: return the URL itself
}

// openRouterHeaders returns the OpenRouter-specific HTTP headers as a map.
func openRouterHeaders() map[string]string {
	h := make(map[string]string)
	if ref := os.Getenv("OPENAI_REFERER"); ref != "" {
		h[headerReferer] = ref
	}
	if title := os.Getenv("OPENAI_APP_TITLE"); title != "" {
		h[headerTitle] = title
	}
	return h
}
