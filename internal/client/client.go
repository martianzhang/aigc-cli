// Package client implements the APIMart API client for image generation.
package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/apimart-cli/internal/types"
)

const (
	defaultBaseURL   = "https://api.apimart.ai"
	imageSubmitPath  = "/v1/images/generations"
	videoSubmitPath  = "/v1/videos/generations"
	chatPath         = "/v1/chat/completions"
	uploadPath       = "/v1/uploads/images"
	taskPath         = "/v1/tasks/%s"
	tokenBalancePath = "/v1/balance"
	userBalancePath  = "/v1/user/balance"
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

	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+imageSubmitPath, bytes.NewReader(body))
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

// ChatCompletion sends a chat request and handles streaming/non-streaming response.
// When req.Stream is true, it prints tokens as they arrive and returns the full response.
// When req.Stream is false, it returns the full response as-is.
func (c *Client) ChatCompletion(req *types.ChatRequest) (*types.ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+chatPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Streaming (SSE)
	if req.Stream {
		return handleSSE(resp)
	}

	// Non-streaming
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result types.ChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

// handleSSE parses SSE stream and prints tokens progressively.
func handleSSE(resp *http.Response) (*types.ChatResponse, error) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer for long lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	full := &types.ChatResponse{
		Choices: []types.ChatChoice{{Message: types.ChatMessage{Role: "assistant"}}},
	}

	var roleSkipped bool
	for scanner.Scan() {
		line := scanner.Text()

		// SSE data lines start with "data: "
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// [DONE] signal
		if data == "[DONE]" {
			break
		}

		var chunk types.ChatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		for _, choice := range chunk.Choices {
			// Skip the first chunk which only contains delta.role="assistant"
			if !roleSkipped && choice.Delta.Role != "" {
				roleSkipped = true
				full.Choices[0].Message.Role = choice.Delta.Role
				// If this chunk also has content, fall through to print it
				if choice.Delta.Content == "" {
					continue
				}
			}

			content := choice.Delta.Content
			if content != "" {
				fmt.Print(content)
				os.Stdout.Sync()
				full.Choices[0].Message.Content += content
			}

			if choice.FinishReason != "" {
				full.Choices[0].FinishReason = choice.FinishReason
			}
		}

		if chunk.ID != "" && full.ID == "" {
			full.ID = chunk.ID
		}
		if chunk.Model != "" && full.Model == "" {
			full.Model = chunk.Model
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("SSE read error: %w", err)
	}

	fmt.Println() // trailing newline after streaming output
	return full, nil
}

// VideoSubmit sends a video generation request and returns the task submission.
func (c *Client) VideoSubmit(req *types.VideoGenerateRequest) (*types.VideoGenerateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.baseURL+videoSubmitPath, bytes.NewReader(body))
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

	var result types.VideoGenerateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

// PollTask polls a task (image or video) until completion or failure.
func (c *Client) PollTask(taskID string) (*types.TaskData, error) {
	fmt.Printf("Task submitted: %s\n", taskID)
	fmt.Printf("Waiting %v before first poll...\n", initialDelay)
	time.Sleep(initialDelay)

	isTTY := isTerminal()
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	si := 0
	start := time.Now()

	// Print initial progress line
	if isTTY {
		fmt.Print("  Progress: 0% ")
	}

	for {
		if time.Since(start) > maxPollDuration {
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("polling timed out after %v", maxPollDuration)
		}

		task, err := c.GetTask(taskID)
		if err != nil {
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("failed to query task: %w", err)
		}

		if isTTY {
			bar := progressBar(task.Progress, 20)
			fmt.Printf("\r  %s %s %d%% ", spinner[si%len(spinner)], bar, task.Progress)
			si++
		} else {
			fmt.Printf("  Status: %s, Progress: %d%%\n", task.Status, task.Progress)
		}

		switch task.Status {
		case "completed":
			if isTTY {
				fmt.Println()
			}
			return task, nil
		case "failed":
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("task %s failed", taskID)
		default:
			// in_progress / submitted — keep polling
		}

		time.Sleep(pollInterval)
	}
}

func isTerminal() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func progressBar(pct, width int) string {
	filled := pct * width / 100
	var bar string
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
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
