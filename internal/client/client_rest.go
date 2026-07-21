package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// HasVersionSuffix reports whether urlStr ends with a version path segment like /v1, /v2.
// Used to avoid duplicating the version when the user-supplied baseURL already includes it.
func HasVersionSuffix(urlStr string) bool {
	lastSlash := strings.LastIndex(urlStr, "/")
	if lastSlash < 0 || lastSlash == len(urlStr)-1 {
		return false
	}
	seg := urlStr[lastSlash+1:]
	if len(seg) < 2 || seg[0] != 'v' {
		return false
	}
	for i := 1; i < len(seg); i++ {
		if seg[i] < '0' || seg[i] > '9' {
			return false
		}
	}
	return true
}

// isLocalFile returns true if the path points to an existing file.
func isLocalFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// timeoutHint returns a user-facing hint when an API request times out.
func timeoutHint() string {
	return `Request timed out.

  If using a sync provider (OpenAI / OpenRouter / third-party relay):
    → Increase timeout: add --timeout <seconds> (e.g. --timeout 300)
    → Or switch to an async provider (APIMart) for resumable tasks

  If using an async provider (APIMart):
    → The task may still be running. Use: aigc-cli task <task-id>`
}

// isTimeoutError checks if an error is caused by an HTTP timeout.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded") || strings.Contains(s, "Client.Timeout")
}

// --- HTTP helpers ---

// doJSON sends a JSON request and unmarshals the response into result.
// If body is nil, sends a request with no body.
func (c *Client) doJSON(method, path string, body, result interface{}) error {
	return c.doJSONWithHeaders(method, path, body, result, nil)
}

// doJSONWithHeaders is like doJSON but with additional HTTP headers.
func (c *Client) doJSONWithHeaders(method, path string, body, result interface{}, extraHeaders map[string]string) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	httpReq, err := http.NewRequestWithContext(c.requestContext(), method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Apply client-level default headers (lowest priority).
	for k, v := range c.defaultHeaders {
		httpReq.Header.Set(k, v)
	}
	// Apply per-request extra headers (override defaults).
	for k, v := range extraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isTimeoutError(err) {
			return fmt.Errorf("API request timed out: %w\n%s", err, timeoutHint())
		}
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}
	return nil
}

// doGet sends a GET request and unmarshals the JSON response into result.
func (c *Client) doGet(path string, result interface{}) error {
	return c.doGetWithHeaders(path, result, nil)
}

// doGetWithHeaders is like doGet but with additional HTTP headers.
func (c *Client) doGetWithHeaders(path string, result interface{}, extraHeaders map[string]string) error {
	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Apply client-level default headers (lowest priority).
	for k, v := range c.defaultHeaders {
		httpReq.Header.Set(k, v)
	}
	// Apply per-request extra headers (override defaults).
	for k, v := range extraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isTimeoutError(err) {
			return fmt.Errorf("request timed out: %w\n%s", err, timeoutHint())
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}
	return nil
}
