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

const (
	anthropicChatPath = "/messages" // Anthropic Messages API endpoint
	anthropicVersion  = "2023-06-01"
)

// anthropicChatCompletion handles chat via the Anthropic Messages API (/v1/messages).
// Converted from the standard ChatRequest to Anthropic format and back.
func (c *Client) anthropicChatCompletion(req *types.ChatRequest) (*types.ChatResponse, error) {
	// Build Anthropic request from standard ChatRequest
	anthropicReq := c.buildAnthropicRequest(req)
	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(c.requestContext(), http.MethodPost, c.baseURL+anthropicChatPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic API request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Streaming mode
	if req.Stream {
		w := req.OutputWriter
		if w == nil {
			w = io.Discard
		}
		return c.handleAnthropicSSE(resp, w)
	}

	// Non-streaming
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read anthropic response: %w", err)
	}

	var msgResp types.AnthropicMessageResponse
	if err := json.Unmarshal(respBody, &msgResp); err != nil {
		return nil, fmt.Errorf("failed to parse anthropic response: %w\nbody: %s", err, truncate(respBody, 200))
	}

	return anthropicToChatResponse(&msgResp), nil
}

// buildAnthropicRequest converts a standard ChatRequest to Anthropic format.
func (c *Client) buildAnthropicRequest(req *types.ChatRequest) *types.AnthropicMessageRequest {
	// Extract system prompt from messages (Anthropic supports system at top level)
	var systemPrompt string
	var msgs []types.AnthropicMessage
	for _, m := range req.Messages {
		if m.Role == "system" {
			systemPrompt = m.Content
			continue
		}
		msgs = append(msgs, types.AnthropicMessage{Role: m.Role, Content: m.Content})
	}

	// Default to empty messages slice if none (Anthropic requires at least one)
	if msgs == nil {
		msgs = []types.AnthropicMessage{}
	}

	maxTokens := 1024
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	ar := &types.AnthropicMessageRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Messages:  msgs,
		System:    systemPrompt,
		Stream:    req.Stream,
	}
	if req.Temperature != nil {
		ar.Temperature = req.Temperature
	}
	return ar
}

// anthropicToChatResponse converts an Anthropic response to the standard ChatResponse.
func anthropicToChatResponse(ar *types.AnthropicMessageResponse) *types.ChatResponse {
	var text string
	for _, block := range ar.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	cr := &types.ChatResponse{
		ID:      ar.ID,
		Object:  "chat.completion",
		Model:   ar.Model,
		Choices: []types.ChatChoice{{Message: types.ChatMessage{Role: "assistant", Content: text}}},
	}
	if ar.Usage != nil {
		cr.Usage = &types.ChatUsage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		}
	}
	return cr
}

// handleAnthropicSSE parses Anthropic's SSE streaming format and writes tokens to w.
// Anthropic SSE uses event types: message_start, content_block_start, content_block_delta,
// content_block_stop, message_delta, message_stop.
func (c *Client) handleAnthropicSSE(resp *http.Response, w io.Writer) (*types.ChatResponse, error) {
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	full := &types.ChatResponse{
		ID:     "",
		Object: "chat.completion",
		Choices: []types.ChatChoice{{
			Index:   0,
			Message: types.ChatMessage{Role: "assistant"},
		}},
	}

	var currentEvent string
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			switch currentEvent {
			case "message_start":
				var start struct {
					Message types.AnthropicMessageResponse `json:"message"`
				}
				if json.Unmarshal([]byte(data), &start) == nil {
					full.ID = start.Message.ID
					full.Model = start.Message.Model
				}

			case "content_block_delta":
				var delta struct {
					Index int `json:"index"`
					Delta struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"delta"`
				}
				if json.Unmarshal([]byte(data), &delta) == nil && delta.Delta.Text != "" {
					full.Choices[0].Message.Content += delta.Delta.Text
					if _, err := fmt.Fprint(w, delta.Delta.Text); err != nil {
						return nil, fmt.Errorf("write stream output: %w", err)
					}
				}

			case "message_delta":
				var msgDelta struct {
					Delta struct {
						StopReason string `json:"stop_reason"`
					} `json:"delta"`
					Usage *struct {
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
				}
				if json.Unmarshal([]byte(data), &msgDelta) == nil {
					if msgDelta.Usage != nil {
						full.Usage = &types.ChatUsage{
							CompletionTokens: msgDelta.Usage.OutputTokens,
						}
					}
				}
			}
			continue
		}

		// Anthropic also sends ping events with data: {"type": "ping"}
		// and message_stop with no useful payload — skip them.
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("anthropic SSE read error: %w", err)
	}

	return full, nil
}
