// Package client implements the APIMart API client for image generation.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/martianzhang/apimart-cli/internal/types"
)

const (
	defaultBaseURL = "https://api.apimart.ai"
	submitPath     = "/v1/images/generations"
	taskPath       = "/v1/tasks/%s"
	// Default polling settings
	pollInterval    = 3 * time.Second
	initialDelay    = 10 * time.Second
	maxPollDuration = 180 * time.Second
)

// Client is the APIMart API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a new API client.
// Pass empty strings for baseURL or proxyURL to use defaults.
// proxyURL supports http://, https://, socks5:// schemes.
// When proxyURL is empty, falls back to HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY env vars.
func New(apiKey, baseURL, proxyURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	transport := &http.Transport{}
	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(parsed)
		}
	} else {
		// Fall back to HTTP_PROXY, HTTPS_PROXY, NO_PROXY, ALL_PROXY env vars
		transport.Proxy = http.ProxyFromEnvironment
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

// Submit sends a generation request and returns the task submission response.
func (c *Client) Submit(req *types.GenerateRequest) (*types.GenerateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+submitPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result types.GenerateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// PollTask polls a task until completion or failure.
func (c *Client) PollTask(taskID string) (*types.TaskData, error) {
	fmt.Printf("Task submitted: %s\n", taskID)
	fmt.Printf("Waiting %v before first poll...\n", initialDelay)
	time.Sleep(initialDelay)

	start := time.Now()
	for {
		if time.Since(start) > maxPollDuration {
			return nil, fmt.Errorf("polling timed out after %v", maxPollDuration)
		}

		task, err := c.GetTask(taskID)
		if err != nil {
			return nil, fmt.Errorf("failed to query task: %w", err)
		}

		fmt.Printf("  Status: %s, Progress: %d%%\n", task.Status, task.Progress)

		switch task.Status {
		case "completed":
			return task, nil
		case "failed":
			return nil, fmt.Errorf("task %s failed", taskID)
		default:
			// in_progress / submitted — keep polling
		}

		time.Sleep(pollInterval)
	}
}

// GetTask retrieves a single task by ID.
func (c *Client) GetTask(taskID string) (*types.TaskData, error) {
	url := c.baseURL + fmt.Sprintf(taskPath, taskID)
	httpReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("task query failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read task response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("task query returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var taskResp types.TaskResponse
	if err := json.Unmarshal(respBody, &taskResp); err != nil {
		return nil, fmt.Errorf("failed to parse task response: %w", err)
	}

	if taskResp.Code != 200 {
		return nil, fmt.Errorf("task query returned code %d", taskResp.Code)
	}

	return taskResp.Data, nil
}
