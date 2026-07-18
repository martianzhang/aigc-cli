package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// the user-supplied value is respected as-is.
func New(apiKey, baseURL, proxyURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	// Normalize: if baseURL doesn't already end with a version path segment like
	// /v1, /v2, /v3, append "/v1" as the default API version for backward
	// compatibility (e.g. bare "https://api.openai.com" → "https://api.openai.com/v1").
	baseURL = strings.TrimRight(baseURL, "/")
	if !HasVersionSuffix(baseURL) {
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

	c := &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   DefaultTimeout,
		},
	}

	// Build default headers sent on every request.
	// Env vars (OPENAI_REFERER, OPENAI_APP_TITLE) take precedence — checked
	// per-request in setOpenRouterHeaders/openRouterHeaders, so defaults here
	// are pure built-in fallbacks.
	headers := map[string]string{
		"User-Agent": "aigc-cli/" + Version,
	}
	if provider.IsOpenRouter(baseURL) {
		// Built-in OpenRouter identity: lets the tool appear in OpenRouter
		// rankings and analytics even when the user hasn't set env vars.
		headers[headerReferer] = "https://github.com/martianzhang/aigc-cli"
		headers[headerTitle] = "aigc-cli"
		headers["X-OpenRouter-Categories"] = "cli-agent"
	}
	c.defaultHeaders = headers

	return c
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
			w = io.Discard // safe fallback; callers should always set OutputWriter
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
