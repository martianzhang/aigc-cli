package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// ResponsesBody returns the OpenAI Responses API request body, or the verbatim
// --json body when set. Shared by the real request and --dry-run.
func ResponsesBody(req *types.ChatRequest) ([]byte, error) {
	if len(req.RawJSON) > 0 {
		return req.RawJSON, nil
	}
	return json.Marshal(buildResponsesRequest(req))
}

// buildResponsesRequest converts a standard ChatRequest to Responses API format.
func buildResponsesRequest(req *types.ChatRequest) map[string]any {
	body := map[string]any{
		"model": req.Model,
	}

	// Extract system messages into instructions
	var instructions []string
	var input []map[string]any
	for _, m := range req.Messages {
		if m.Role == "system" {
			instructions = append(instructions, m.Content)
			continue
		}
		input = append(input, translateMessageToResponsesItem(m)...)
	}
	if len(instructions) > 0 {
		body["instructions"] = strings.Join(instructions, "\n\n")
	}
	body["input"] = input
	body["stream"] = req.Stream

	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		body["max_output_tokens"] = *req.MaxTokens
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if len(req.Tools) > 0 {
		body["tools"] = buildResponsesTools(req.Tools)
	}

	return body
}

// translateMessageToResponsesItem converts a ChatMessage to one or more
// Responses API input items.
func translateMessageToResponsesItem(m types.ChatMessage) []map[string]any {
	if m.Role == "tool" {
		return []map[string]any{{
			"type":    "function_call_output",
			"call_id": m.ToolCallID,
			"output":  m.Content,
		}}
	}

	if m.Role == "assistant" && len(m.ToolCalls) > 0 {
		items := make([]map[string]any, 0, len(m.ToolCalls)+1)
		if m.Content != "" {
			items = append(items, map[string]any{
				"role":    "assistant",
				"content": m.Content,
			})
		}
		for _, tc := range m.ToolCalls {
			items = append(items, map[string]any{
				"type":      "function_call",
				"call_id":   tc.ID,
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			})
		}
		return items
	}

	item := map[string]any{"role": m.Role}
	if m.Content != "" {
		item["content"] = m.Content
	}
	return []map[string]any{item}
}

// buildResponsesTools converts ToolDefinitions to the flat Responses API shape.
func buildResponsesTools(tools []types.ToolDefinition) []map[string]any {
	out := make([]map[string]any, len(tools))
	for i, td := range tools {
		out[i] = map[string]any{
			"type":        "function",
			"name":        td.Function.Name,
			"description": td.Function.Description,
			"parameters":  json.RawMessage(td.Function.Parameters),
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Responses API wire-format structs
// ---------------------------------------------------------------------------

type responsesResponse struct {
	ID     string                `json:"id"`
	Model  string                `json:"model"`
	Output []responsesOutputItem `json:"output"`
	Usage  *responsesUsage       `json:"usage,omitempty"`
	Status string                `json:"status"`
}

type responsesOutputItem struct {
	Type      string             `json:"type"`
	ID        string             `json:"id,omitempty"`
	Role      string             `json:"role,omitempty"`
	CallID    string             `json:"call_id,omitempty"`
	Name      string             `json:"name,omitempty"`
	Arguments string             `json:"arguments,omitempty"`
	Content   []responsesContent `json:"content,omitempty"`
}

type responsesContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ---------------------------------------------------------------------------
// responsesChatCompletion
// ---------------------------------------------------------------------------

func (c *Client) responsesChatCompletion(req *types.ChatRequest) (*types.ChatResponse, error) {
	body, err := ResponsesBody(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responses request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+ResponsesPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create responses request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	c.setOpenRouterHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("responses API request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("responses API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if req.Stream {
		w := req.OutputWriter
		if w == nil {
			w = io.Discard
		}
		return c.handleResponsesSSE(resp, w)
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read responses response: %w", err)
	}

	// Some providers return SSE even when stream=false
	trimmed := bytes.TrimSpace(respBody)
	if bytes.HasPrefix(trimmed, []byte("data: ")) || bytes.HasPrefix(trimmed, []byte("event: ")) || bytes.HasPrefix(trimmed, []byte(":")) {
		fakeResp := &http.Response{
			Body:       io.NopCloser(bytes.NewReader(respBody)),
			StatusCode: http.StatusOK,
		}
		return c.handleResponsesSSE(fakeResp, io.Discard)
	}

	var result responsesResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse responses response: %w\nbody: %s", err, truncate(respBody, 200))
	}
	return responsesToChatResponse(&result), nil
}

// responsesToChatResponse converts a Responses API response to ChatResponse.
func responsesToChatResponse(r *responsesResponse) *types.ChatResponse {
	msg := types.ChatMessage{Role: "assistant"}
	var toolCalls []types.ToolCall

	for _, item := range r.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" {
					msg.Content += c.Text
				}
			}
		case "function_call":
			toolCalls = append(toolCalls, types.ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: types.ToolCallFunction{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
		msg.ToolCalls = toolCalls
	}

	cr := &types.ChatResponse{
		ID:      r.ID,
		Object:  "chat.completion",
		Model:   r.Model,
		Choices: []types.ChatChoice{{Index: 0, Message: msg, FinishReason: finishReason}},
	}
	if r.Usage != nil {
		cr.Usage = &types.ChatUsage{
			PromptTokens:     r.Usage.InputTokens,
			CompletionTokens: r.Usage.OutputTokens,
			TotalTokens:      r.Usage.TotalTokens,
		}
	}
	return cr
}

// ---------------------------------------------------------------------------
// Responses SSE streaming
// ---------------------------------------------------------------------------

// accumulatedFunctionCall tracks a function_call item during streaming.
type accumulatedFunctionCall struct {
	callID    string
	name      string
	arguments string
}

func (c *Client) handleResponsesSSE(resp *http.Response, w io.Writer) (*types.ChatResponse, error) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var contentBuf strings.Builder
	var funcCalls map[string]*accumulatedFunctionCall
	var funcCallOrder []string
	var lastFuncCallID string
	var completed *responsesResponse
	var id, model string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event struct {
			Type     string               `json:"type"`
			Delta    string               `json:"delta"`
			ItemID   string               `json:"item_id"`
			Item     *responsesOutputItem `json:"item"`
			Response *responsesResponse   `json:"response"`
			Error    *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				fmt.Fprint(w, event.Delta)
				if s, ok := w.(interface{ Sync() error }); ok {
					s.Sync()
				}
				contentBuf.WriteString(event.Delta)
			}

		case "response.output_item.added", "response.output_item.done":
			if event.Item != nil && event.Item.Type == "function_call" {
				if funcCalls == nil {
					funcCalls = make(map[string]*accumulatedFunctionCall)
				}
				itemID := event.Item.ID
				if itemID == "" {
					itemID = event.ItemID
				}
				afc := &accumulatedFunctionCall{
					callID: event.Item.CallID,
					name:   event.Item.Name,
				}
				if itemID != "" {
					if _, exists := funcCalls[itemID]; !exists {
						funcCallOrder = append(funcCallOrder, itemID)
					}
					funcCalls[itemID] = afc
					lastFuncCallID = itemID
				} else if lastFuncCallID != "" {
					// No ID on this event: update the most recent function call.
					funcCalls[lastFuncCallID] = afc
				}
			}

		case "response.function_call_arguments.delta":
			if event.Delta != "" {
				if event.ItemID != "" && funcCalls != nil {
					if fc, ok := funcCalls[event.ItemID]; ok {
						fc.arguments += event.Delta
						lastFuncCallID = event.ItemID
					} else {
						// Unknown item_id: append to last seen function_call
						if lastFuncCallID != "" {
							funcCalls[lastFuncCallID].arguments += event.Delta
						}
					}
				} else if lastFuncCallID != "" && funcCalls != nil {
					funcCalls[lastFuncCallID].arguments += event.Delta
				}
			}

		case "response.completed":
			if event.Response != nil {
				completed = event.Response
				if completed.ID != "" {
					id = completed.ID
				}
				if completed.Model != "" {
					model = completed.Model
				}
			}

		case "response.failed":
			if event.Error != nil {
				return nil, fmt.Errorf("responses API failed: %s", event.Error.Message)
			}
			return nil, fmt.Errorf("responses API failed")

		case "error":
			if event.Error != nil {
				return nil, fmt.Errorf("responses API error: %s", event.Error.Message)
			}
			return nil, fmt.Errorf("responses API error")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("responses SSE read error: %w", err)
	}

	// Print trailing newline if any text content was written.
	if contentBuf.Len() > 0 {
		fmt.Fprintln(w)
	}

	if completed != nil {
		return responsesToChatResponse(completed), nil
	}

	// Synthesize from accumulated buffers
	msg := types.ChatMessage{Role: "assistant", Content: contentBuf.String()}
	var toolCalls []types.ToolCall
	for _, itemID := range funcCallOrder {
		fc := funcCalls[itemID]
		if fc == nil {
			continue
		}
		toolCalls = append(toolCalls, types.ToolCall{
			ID:   fc.callID,
			Type: "function",
			Function: types.ToolCallFunction{
				Name:      fc.name,
				Arguments: fc.arguments,
			},
		})
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	return &types.ChatResponse{
		ID:      id,
		Object:  "chat.completion",
		Model:   model,
		Choices: []types.ChatChoice{{Index: 0, Message: msg, FinishReason: finishReason}},
	}, nil
}
