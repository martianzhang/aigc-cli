// Package client implements the APIMart API client for image generation.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/martianzhang/apimart-cli/internal/types"
)

const (
	defaultBaseURL    = "https://api.apimart.ai"
	submitPath        = "/v1/images/generations"
	uploadPath        = "/v1/uploads/images"
	taskPath          = "/v1/tasks/%s"
	tokenBalancePath  = "/v1/balance"
	userBalancePath   = "/v1/user/balance"
	// Default polling settings
	pollInterval    = 3 * time.Second
	initialDelay    = 10 * time.Second
	maxPollDuration = 180 * time.Second
	uploadTimeout   = 60 * time.Second
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

// UploadImage uploads a local image file and returns the public URL.
func (c *Client) UploadImage(filePath string) (*types.UploadResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(fw, file); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}
	w.Close()

	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+uploadPath, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Uploads may take longer
	oldTimeout := c.httpClient.Timeout
	c.httpClient.Timeout = uploadTimeout
	defer func() { c.httpClient.Timeout = oldTimeout }()

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result types.UploadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse upload response: %w", err)
	}
	return &result, nil
}

// ResolveLocalImages checks each URL; if it's a local file path, uploads it
// and returns the public URL. Unchanged URLs are returned as-is.
func (c *Client) ResolveLocalImages(urls []string) ([]string, error) {
	resolved := make([]string, len(urls))
	for i, u := range urls {
		if isLocalFile(u) {
			fmt.Printf("  Uploading local file: %s ...\n", u)
			resp, err := c.UploadImage(u)
			if err != nil {
				return nil, fmt.Errorf("failed to upload %s: %w", u, err)
			}
			fmt.Printf("  -> %s\n", resp.URL)
			resolved[i] = resp.URL
		} else {
			resolved[i] = u
		}
	}
	return resolved, nil
}

// GetTokenBalance queries the current token's balance.
func (c *Client) GetTokenBalance() (*types.TokenBalanceResponse, error) {
	return getBalance[types.TokenBalanceResponse](c, c.baseURL+tokenBalancePath)
}

// GetUserBalance queries the current user's balance.
func (c *Client) GetUserBalance() (*types.UserBalanceResponse, error) {
	return getBalance[types.UserBalanceResponse](c, c.baseURL+userBalancePath)
}

// getBalance is a generic helper for balance endpoints.
func getBalance[T any](c *Client, url string) (*T, error) {
	httpReq, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result T
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

// isLocalFile returns true if the path points to an existing file.
func isLocalFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
