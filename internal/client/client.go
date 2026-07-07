// Package client implements API client for image generation, chat, and more.
// Supports OpenAI-compatible (sync) and APIMart (async task) backends.
package client

import (
	"bufio"
	"bytes"
	"context"
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

	"github.com/martianzhang/apimart-cli/internal/provider"
	"github.com/martianzhang/apimart-cli/internal/types"
)

const (
	defaultBaseURL    = "https://api.apimart.ai"
	imageSubmitPath   = "/images/generations"
	videoSubmitPath   = "/videos/generations"
	yunwuVideoSubPath = "/video/create"
	yunwuVideoQryPath = "/video/query"
	chatPath          = "/chat/completions"
	uploadPath        = "/uploads/images"
	taskPath          = "/tasks/%s"
	tokenBalancePath  = "/balance"
	userBalancePath   = "/user/balance"
	modelsPath        = "/models"
	// OpenRouter-specific header names
	headerReferer = "HTTP-Referer"
	headerTitle   = "X-OpenRouter-Title"
	// Default polling settings
	pollInterval    = 3 * time.Second
	initialDelay    = 10 * time.Second
	maxPollDuration = 180 * time.Second
	uploadTimeout   = 60 * time.Second
	// Default HTTP client timeout for API requests (used as initial value for DefaultTimeout)
	defaultHTTPTimeout = 180 * time.Second
	// Modality-specific HTTP timeouts (exported for use by cmd/ commands)
	ImageTimeout = 180 * time.Second
	VideoTimeout = 600 * time.Second
	MJTimeout    = 600 * time.Second
)

// DefaultTimeout is the timeout used by New(). Commands can override this
// before creating clients (safe for single-threaded CLI usage).
var DefaultTimeout = defaultHTTPTimeout

// Client is the API client for image generation, chat, and more.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	ctx        context.Context // optional, for cancellation (set by SetContext)
}

// SetContext sets an optional context for cancellation support.
// When set, all HTTP requests and polling loops respect ctx.Done().
// Thread-safe for single-threaded CLI usage.
func (c *Client) SetContext(ctx context.Context) {
	c.ctx = ctx
}

// requestContext returns the client's context or context.Background().
func (c *Client) requestContext() context.Context {
	return c.GetContext()
}

// GetContext returns the client's context or context.Background().
// Exported so cmd/ packages can propagate context to new clients.
func (c *Client) GetContext() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

// SetTimeout sets the HTTP client timeout. Use 0 for no timeout.
func (c *Client) SetTimeout(d time.Duration) {
	c.httpClient.Timeout = d
}

// New creates a new API client.
// Pass empty strings for baseURL or proxyURL to use defaults.
// proxyURL supports http://, https://, socks5:// schemes.
// When proxyURL is empty, falls back to HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY env vars.
//
// baseURL should include the API version prefix (e.g. "https://api.apimart.ai/v1").
// For backward compatibility, if baseURL doesn't end with a "/vN" path segment,
// "/v1" is appended automatically (so "https://api.openai.com" → "https://api.openai.com/v1").
// If it already includes a version (e.g. "https://relay.com/v2", "https://openrouter.ai/api/v1"),
// the user-supplied value is respected as-is.
func New(apiKey, baseURL, proxyURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	// Normalize: if baseURL doesn't already end with a version path segment like
	// /v1, /v2, /v3, append "/v1" as the default API version for backward
	// compatibility (e.g. bare "https://api.openai.com" → "https://api.openai.com/v1").
	baseURL = strings.TrimRight(baseURL, "/")
	if !hasVersionSuffix(baseURL) {
		baseURL += "/v1"
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
			Timeout:   DefaultTimeout,
		},
	}
}

// Submit sends a generation request and returns the task submission response.
func (c *Client) Submit(req *types.GenerateRequest) (*types.GenerateResponse, error) {
	var result types.GenerateResponse
	if err := c.doJSON(http.MethodPost, imageSubmitPath, req, &result); err != nil {
		return nil, err
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

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+chatPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	c.setOpenRouterHeaders(httpReq)

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
		w := req.OutputWriter
		if w == nil {
			w = os.Stdout // backward compatibility: default to stdout
		}
		return handleSSE(resp, w)
	}

	// Non-streaming — but some providers (e.g. APIMart.ai) always return SSE format
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Detect SSE format: response starts with "data: " or ":" (keepalive comment)
	trimmed := bytes.TrimSpace(respBody)
	if bytes.HasPrefix(trimmed, []byte("data: ")) || bytes.HasPrefix(trimmed, []byte(":")) {
		fakeResp := &http.Response{
			Body:       io.NopCloser(bytes.NewReader(respBody)),
			StatusCode: http.StatusOK,
		}
		return handleSSE(fakeResp, io.Discard)
	}

	var result types.ChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w\nbody: %s", err, truncate(respBody, 200))
	}
	return &result, nil
}

// truncate returns the first n bytes of b as a string, with "..." if truncated.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

// handleSSE parses SSE stream and writes tokens progressively to w.
// Supports text content and tool_calls delta accumulation.
func handleSSE(resp *http.Response, w io.Writer) (*types.ChatResponse, error) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer for long lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	full := &types.ChatResponse{
		Choices: []types.ChatChoice{{Message: types.ChatMessage{Role: "assistant"}}},
	}

	// Accumulate tool calls by index across streaming chunks
	// Key: tool call index, Value: accumulated ToolCall
	toolCallAccum := map[int]*types.ToolCall{}

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
				// If this chunk has neither content nor tool_calls, skip
				if choice.Delta.Content == "" && len(choice.Delta.ToolCalls) == 0 {
					continue
				}
			}

			// Accumulate tool call deltas
			for _, tc := range choice.Delta.ToolCalls {
				acc, exists := toolCallAccum[tc.Index]
				if !exists {
					acc = &types.ToolCall{
						Type: "function",
					}
					toolCallAccum[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Type != "" {
					acc.Type = tc.Type
				}
				if tc.Function != nil {
					if tc.Function.Name != "" {
						acc.Function.Name += tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						acc.Function.Arguments += tc.Function.Arguments
					}
				}
			}

			// Write text content (only if no tool calls in this response)
			content := choice.Delta.Content
			if content != "" {
				fmt.Fprint(w, content)
				// Sync the writer if it supports it (e.g., os.Stdout)
				if s, ok := w.(interface{ Sync() error }); ok {
					s.Sync()
				}
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

	// Flush accumulated tool calls into the response message
	if len(toolCallAccum) > 0 {
		tcs := make([]types.ToolCall, 0, len(toolCallAccum))
		for i := 0; i < len(toolCallAccum); i++ {
			if tc, ok := toolCallAccum[i]; ok {
				tcs = append(tcs, *tc)
			}
		}
		full.Choices[0].Message.ToolCalls = tcs
	}

	// Only print trailing newline if we wrote text content
	if full.Choices[0].Message.Content != "" {
		fmt.Fprintln(w)
	}
	return full, nil
}

// VideoSubmit sends a video generation request and returns the task submission.
func (c *Client) VideoSubmit(req *types.VideoGenerateRequest) (*types.VideoGenerateResponse, error) {
	var result types.VideoGenerateResponse
	if err := c.doJSON(http.MethodPost, videoSubmitPath, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

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

// VideoRemixSubmit sends a VEO3 remix request to POST /v1/videos/{task_id}/remix.
func (c *Client) VideoRemixSubmit(taskID string, req *types.VideoRemixRequest) (*types.VideoRemixResponse, error) {
	path := fmt.Sprintf("/videos/%s/remix", taskID)
	var result types.VideoRemixResponse
	if err := c.doJSON(http.MethodPost, path, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PollTask polls a task (image or video) until completion or failure.
func (c *Client) PollTask(taskID string) (*types.TaskData, error) {
	fmt.Printf("Task submitted: %s\n", taskID)
	fmt.Printf("Waiting %v before first poll...\n", initialDelay)
	select {
	case <-time.After(initialDelay):
	case <-c.requestContext().Done():
		return nil, c.requestContext().Err()
	}

	isTTY := isTerminal()
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	si := 0
	start := time.Now()

	// Print initial progress line
	if isTTY {
		fmt.Print("  Progress: 0% ")
	}

	for {
		// Check for cancellation before each poll cycle
		select {
		case <-c.requestContext().Done():
			if isTTY {
				fmt.Println()
			}
			return nil, c.requestContext().Err()
		default:
		}

		if time.Since(start) > maxPollDuration {
			if isTTY {
				fmt.Println()
			}
			return nil, fmt.Errorf("polling timed out after %v\n  The task may still be running. Use: aigc-cli task %s", maxPollDuration, taskID)
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
		case "completed", "success", "succeeded":
			if isTTY {
				fmt.Println()
			}
			return task, nil
		case "failed", "failure":
			if isTTY {
				fmt.Println()
			}
			if task.Error != nil && task.Error.Message != "" {
				return nil, fmt.Errorf("task %s failed: %s", taskID, task.Error.Message)
			}
			return nil, fmt.Errorf("task %s failed", taskID)
		default:
			// in_progress / submitted / processing — keep polling
		}

		// Sleep with cancellation support
		select {
		case <-time.After(pollInterval):
		case <-c.requestContext().Done():
			if isTTY {
				fmt.Println()
			}
			return nil, c.requestContext().Err()
		}
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
	path := fmt.Sprintf(taskPath, taskID)

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodGet, c.baseURL+path, nil)
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

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+uploadPath, &buf)
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
	return getBalance[types.TokenBalanceResponse](c, tokenBalancePath)
}

// GetUserBalance queries the current user's balance.
func (c *Client) GetUserBalance() (*types.UserBalanceResponse, error) {
	return getBalance[types.UserBalanceResponse](c, userBalancePath)
}

// getBalance is a generic helper for balance endpoints.
// path should be relative (without baseURL), doGet prepends it.
func getBalance[T any](c *Client, path string) (*T, error) {
	var result T
	if err := c.doGet(path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// --- Provider detection ---

// IsAPIMartProvider returns true if the base URL points to an APIMart-provided domain.
func (c *Client) IsAPIMartProvider() bool {
	return provider.IsAPIMart(c.baseURL)
}

// IsOpenRouterProvider returns true if the base URL points to OpenRouter.
func (c *Client) IsOpenRouterProvider() bool {
	return provider.IsOpenRouter(c.baseURL)
}

// --- Sync image generation (OpenAI / OpenRouter compatible) ---

// ImageGenerateSync sends a synchronous image generation request compatible with
// OpenAI and OpenRouter. Returns the response with image URLs directly.
func (c *Client) ImageGenerateSync(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	var result types.OpenAIImageResponse
	if err := c.doJSON(http.MethodPost, imageSubmitPath, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// --- Balance ---

// --- Models (OpenAI-compatible) ---

// ListModelsOpenAI fetches the model list from OpenAI-compatible /v1/models endpoint.
func (c *Client) ListModelsOpenAI() ([]types.OpenAIModel, error) {
	var result struct {
		Data []types.OpenAIModel `json:"data"`
	}
	if err := c.doGet(modelsPath, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetModelOpenAI fetches a single model by ID from the OpenAI-compatible /v1/models/{model} endpoint.
func (c *Client) GetModelOpenAI(modelID string) (*types.OpenAIModel, error) {
	path := modelsPath + "/" + modelID
	var result types.OpenAIModel
	if err := c.doGet(path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// --- Helpers ---

// setOpenRouterHeaders adds optional OpenRouter-specific headers.
// Reads environment variables directly to avoid storing provider-specific state.
func (c *Client) setOpenRouterHeaders(req *http.Request) {
	if ref := os.Getenv("OPENAI_REFERER"); ref != "" {
		req.Header.Set(headerReferer, ref)
	}
	if title := os.Getenv("OPENAI_APP_TITLE"); title != "" {
		req.Header.Set(headerTitle, title)
	}
}

// hasVersionSuffix checks if urlStr ends with a version path segment like /v1, /v2, /v3.
// Used to avoid duplicating the version when the user-supplied baseURL already includes it.
func hasVersionSuffix(urlStr string) bool {
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
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

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
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

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
