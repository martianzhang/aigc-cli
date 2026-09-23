package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ---------------------------------------------------------------------------
// ResponsesBody tests
// ---------------------------------------------------------------------------

func TestResponsesBody_rawJSONForwarded(t *testing.T) {
	const raw = `{"model":"gpt-6-luna","input":[{"role":"user","content":"hi"}]}`
	got, err := ResponsesBody(&types.ChatRequest{RawJSON: []byte(raw)})
	if err != nil {
		t.Fatalf("ResponsesBody: %v", err)
	}
	if string(got) != raw {
		t.Errorf("raw body = %s, want %s", got, raw)
	}
}

func TestResponsesBody_systemBecomesInstructions(t *testing.T) {
	req := &types.ChatRequest{
		Model: "gpt-6-luna",
		Messages: []types.ChatMessage{
			{Role: "system", Content: "You are helpful."},
			{Role: "system", Content: "Be concise."},
			{Role: "user", Content: "Hello"},
		},
	}
	got, err := ResponsesBody(req)
	if err != nil {
		t.Fatalf("ResponsesBody: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["instructions"] != "You are helpful.\n\nBe concise." {
		t.Errorf("instructions = %q, want %q", body["instructions"], "You are helpful.\n\nBe concise.")
	}
}

func TestResponsesBody_userMessage(t *testing.T) {
	req := &types.ChatRequest{
		Model:    "gpt-6-luna",
		Messages: []types.ChatMessage{{Role: "user", Content: "Hello"}},
	}
	got, err := ResponsesBody(req)
	if err != nil {
		t.Fatalf("ResponsesBody: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	input, ok := body["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %+v, want 1 item", body["input"])
	}
	item := input[0].(map[string]any)
	if item["role"] != "user" || item["content"] != "Hello" {
		t.Errorf("item = %+v, want user/Hello", item)
	}
}

func TestResponsesBody_assistantToolCalls(t *testing.T) {
	req := &types.ChatRequest{
		Model: "gpt-6-luna",
		Messages: []types.ChatMessage{
			{
				Role:    "assistant",
				Content: "Let me check that.",
				ToolCalls: []types.ToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: types.ToolCallFunction{
							Name:      "get_weather",
							Arguments: `{"city":"NYC"}`,
						},
					},
				},
			},
		},
	}
	got, err := ResponsesBody(req)
	if err != nil {
		t.Fatalf("ResponsesBody: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	input, _ := body["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("input length = %d, want 2", len(input))
	}
	first := input[0].(map[string]any)
	if first["role"] != "assistant" || first["content"] != "Let me check that." {
		t.Errorf("first item = %+v", first)
	}
	second := input[1].(map[string]any)
	if second["type"] != "function_call" || second["call_id"] != "call_1" || second["name"] != "get_weather" || second["arguments"] != `{"city":"NYC"}` {
		t.Errorf("second item = %+v", second)
	}
}

func TestResponsesBody_toolMessage(t *testing.T) {
	req := &types.ChatRequest{
		Model: "gpt-6-luna",
		Messages: []types.ChatMessage{
			{Role: "tool", Content: `{"temp":72}`, ToolCallID: "call_1"},
		},
	}
	got, err := ResponsesBody(req)
	if err != nil {
		t.Fatalf("ResponsesBody: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	input, _ := body["input"].([]any)
	if len(input) != 1 {
		t.Fatalf("input length = %d, want 1", len(input))
	}
	item := input[0].(map[string]any)
	if item["type"] != "function_call_output" || item["call_id"] != "call_1" || item["output"] != `{"temp":72}` {
		t.Errorf("item = %+v", item)
	}
}

func TestResponsesBody_toolsFlatShape(t *testing.T) {
	req := &types.ChatRequest{
		Model: "gpt-6-luna",
		Messages: []types.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Tools: []types.ToolDefinition{
			{
				Type: "function",
				Function: types.ToolFunction{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			},
		},
	}
	got, err := ResponsesBody(req)
	if err != nil {
		t.Fatalf("ResponsesBody: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tools, _ := body["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools length = %d, want 1", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["type"] != "function" {
		t.Errorf("tool.type = %q, want function", tool["type"])
	}
	if tool["name"] != "get_weather" {
		t.Errorf("tool.name = %q, want get_weather", tool["name"])
	}
	if tool["description"] != "Get weather" {
		t.Errorf("tool.description = %q, want Get weather", tool["description"])
	}
	if _, hasFunctionKey := tool["function"]; hasFunctionKey {
		t.Error("tool should NOT have nested 'function' key")
	}
}

func TestResponsesBody_maxOutputTokens(t *testing.T) {
	mtok := 512
	req := &types.ChatRequest{
		Model:     "gpt-6-luna",
		Messages:  []types.ChatMessage{{Role: "user", Content: "Hi"}},
		MaxTokens: &mtok,
	}
	got, err := ResponsesBody(req)
	if err != nil {
		t.Fatalf("ResponsesBody: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["max_output_tokens"] != float64(512) {
		t.Errorf("max_output_tokens = %v, want 512", body["max_output_tokens"])
	}
	if _, hasMaxTokens := body["max_tokens"]; hasMaxTokens {
		t.Error("should NOT have max_tokens key")
	}
}

func TestResponsesBody_omitsOptionalFields(t *testing.T) {
	req := &types.ChatRequest{
		Model:    "gpt-6-luna",
		Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
	}
	got, err := ResponsesBody(req)
	if err != nil {
		t.Fatalf("ResponsesBody: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"temperature", "top_p", "tools", "instructions"} {
		if _, ok := body[key]; ok {
			t.Errorf("should not have %s when not set", key)
		}
	}
}

// ---------------------------------------------------------------------------
// Non-streaming tests
// ---------------------------------------------------------------------------

func TestResponsesChatCompletion_nonStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/v1" + ResponsesPath
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer sk-responses" {
			t.Errorf("Authorization = %q, want Bearer sk-responses", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "resp_01",
			"model": "gpt-6-luna",
			"status": "completed",
			"output": [
				{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "The weather is sunny."}]},
				{"type": "function_call", "call_id": "call_1", "name": "get_weather", "arguments": "{\"city\":\"NYC\"}"}
			],
			"usage": {"input_tokens": 10, "output_tokens": 20, "total_tokens": 30}
		}`))
	}))
	defer srv.Close()

	c := NewWithProvider("sk-responses", srv.URL, "", types.ProviderOpenAIResponses)
	req := &types.ChatRequest{
		Model:    "gpt-6-luna",
		Messages: []types.ChatMessage{{Role: "user", Content: "What's the weather?"}},
	}
	resp, err := c.ChatCompletion(req)
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if resp.ID != "resp_01" {
		t.Errorf("ID = %q, want resp_01", resp.ID)
	}
	if resp.Model != "gpt-6-luna" {
		t.Errorf("Model = %q, want gpt-6-luna", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Choices = %d, want 1", len(resp.Choices))
	}
	msg := resp.Choices[0].Message
	if msg.Content != "The weather is sunny." {
		t.Errorf("Content = %q, want The weather is sunny.", msg.Content)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d, want 1", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "call_1" {
		t.Errorf("ToolCall.ID = %q, want call_1", msg.ToolCalls[0].ID)
	}
	if msg.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("ToolCall.Name = %q, want get_weather", msg.ToolCalls[0].Function.Name)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", resp.Choices[0].FinishReason)
	}
	if resp.Usage == nil {
		t.Fatal("Usage is nil")
	}
	if resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 20 || resp.Usage.TotalTokens != 30 {
		t.Errorf("Usage = %+v, want 10/20/30", resp.Usage)
	}
}

// ---------------------------------------------------------------------------
// Streaming tests
// ---------------------------------------------------------------------------

func TestResponsesChatCompletion_streaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("not a flusher")
		}
		events := []string{
			`data: {"type":"response.output_text.delta","delta":"Hello"}`,
			`data: {"type":"response.output_text.delta","delta":" world"}`,
			`data: {"type":"response.completed","response":{"id":"resp_02","model":"gpt-6-luna","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello world"}]}],"usage":{"input_tokens":5,"output_tokens":10,"total_tokens":15}}}`,
		}
		for _, ev := range events {
			fmt.Fprintln(w, ev)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	var buf bytes.Buffer
	c := NewWithProvider("sk-responses", srv.URL, "", types.ProviderOpenAIResponses)
	req := &types.ChatRequest{
		Model:        "gpt-6-luna",
		Messages:     []types.ChatMessage{{Role: "user", Content: "Hi"}},
		Stream:       true,
		OutputWriter: &buf,
	}
	resp, err := c.ChatCompletion(req)
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if buf.String() != "Hello world\n" {
		t.Errorf("streamed text = %q, want \"Hello world\\n\"", buf.String())
	}
	if resp.ID != "resp_02" {
		t.Errorf("ID = %q, want resp_02", resp.ID)
	}
	if resp.Model != "gpt-6-luna" {
		t.Errorf("Model = %q, want gpt-6-luna", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Choices = %d, want 1", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello world" {
		t.Errorf("Content = %q, want Hello world", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", resp.Choices[0].FinishReason)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 5 {
		t.Errorf("Usage = %+v, want prompt=5", resp.Usage)
	}
}

func TestResponsesChatCompletion_streamingToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("not a flusher")
		}
		events := []string{
			`data: {"type":"response.output_item.added","item_id":"item_1","item":{"type":"function_call","id":"item_1","call_id":"call_2","name":"get_weather","arguments":""}}`,
			`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"city\":\"NYC\"}"}`,
			`data: {"type":"response.output_item.done","item_id":"item_1","item":{"type":"function_call","id":"item_1","call_id":"call_2","name":"get_weather","arguments":"{\"city\":\"NYC\"}"}}`,
			`data: {"type":"response.completed","response":{"id":"resp_03","model":"gpt-6-luna","output":[{"type":"function_call","call_id":"call_2","name":"get_weather","arguments":"{\"city\":\"NYC\"}"}],"usage":{"input_tokens":8,"output_tokens":12,"total_tokens":20}}}`,
		}
		for _, ev := range events {
			fmt.Fprintln(w, ev)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	var buf bytes.Buffer
	c := NewWithProvider("sk-responses", srv.URL, "", types.ProviderOpenAIResponses)
	req := &types.ChatRequest{
		Model:        "gpt-6-luna",
		Messages:     []types.ChatMessage{{Role: "user", Content: "Weather?"}},
		Stream:       true,
		OutputWriter: &buf,
	}
	resp, err := c.ChatCompletion(req)
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if buf.String() != "" {
		t.Errorf("streamed text should be empty for tool-only response, got %q", buf.String())
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Choices = %d, want 1", len(resp.Choices))
	}
	msg := resp.Choices[0].Message
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d, want 1", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "call_2" {
		t.Errorf("ToolCall.ID = %q, want call_2", msg.ToolCalls[0].ID)
	}
	if msg.ToolCalls[0].Function.Arguments != `{"city":"NYC"}` {
		t.Errorf("ToolCall.Arguments = %q, want {\"city\":\"NYC\"}", msg.ToolCalls[0].Function.Arguments)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want tool_calls", resp.Choices[0].FinishReason)
	}
}

func TestResponsesChatCompletion_streamingSynthesizeWithoutCompleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("not a flusher")
		}
		events := []string{
			`data: {"type":"response.output_text.delta","delta":"Hi"}`,
			`data: {"type":"response.output_text.delta","delta":" there"}`,
		}
		for _, ev := range events {
			fmt.Fprintln(w, ev)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	var buf bytes.Buffer
	c := NewWithProvider("sk-responses", srv.URL, "", types.ProviderOpenAIResponses)
	req := &types.ChatRequest{
		Model:        "gpt-6-luna",
		Messages:     []types.ChatMessage{{Role: "user", Content: "Hi"}},
		Stream:       true,
		OutputWriter: &buf,
	}
	resp, err := c.ChatCompletion(req)
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if buf.String() != "Hi there\n" {
		t.Errorf("streamed text = %q, want \"Hi there\\n\"", buf.String())
	}
	if resp.Choices[0].Message.Content != "Hi there" {
		t.Errorf("Content = %q, want Hi there", resp.Choices[0].Message.Content)
	}
}

func TestResponsesChatCompletion_sseDetectionWhenStreamFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_sse\",\"model\":\"gpt-6-luna\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}]}}\n"))
	}))
	defer srv.Close()

	c := NewWithProvider("sk-responses", srv.URL, "", types.ProviderOpenAIResponses)
	req := &types.ChatRequest{
		Model:    "gpt-6-luna",
		Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
		Stream:   false,
	}
	resp, err := c.ChatCompletion(req)
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if resp.ID != "resp_sse" {
		t.Errorf("ID = %q, want resp_sse", resp.ID)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Errorf("Content = %q, want ok", resp.Choices[0].Message.Content)
	}
}

// ---------------------------------------------------------------------------
// Factory routing test
// ---------------------------------------------------------------------------

func TestNewFromProvider_responses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{
		Type:    types.ProviderOpenAIResponses,
		BaseURL: srv.URL,
		APIKey:  "sk-responses-test",
	}
	c := NewFromProvider(p)
	if c.providerType != types.ProviderOpenAIResponses {
		t.Errorf("providerType = %q, want %q", c.providerType, types.ProviderOpenAIResponses)
	}
	if c.apiKey != "sk-responses-test" {
		t.Errorf("apiKey = %q, want sk-responses-test", c.apiKey)
	}
}

// ---------------------------------------------------------------------------
// responsesToChatResponse edge cases
// ---------------------------------------------------------------------------

func TestResponsesToChatResponse_emptyOutput(t *testing.T) {
	r := &responsesResponse{ID: "r1", Model: "m", Output: []responsesOutputItem{}}
	got := responsesToChatResponse(r)
	if got.Choices[0].Message.Content != "" {
		t.Errorf("Content = %q, want empty", got.Choices[0].Message.Content)
	}
	if got.Choices[0].FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", got.Choices[0].FinishReason)
	}
}

func TestResponsesToChatResponse_textOnly(t *testing.T) {
	r := &responsesResponse{
		ID:    "r2",
		Model: "m",
		Output: []responsesOutputItem{
			{Type: "message", Content: []responsesContent{{Type: "output_text", Text: "Hello"}}},
		},
	}
	got := responsesToChatResponse(r)
	if got.Choices[0].Message.Content != "Hello" {
		t.Errorf("Content = %q, want Hello", got.Choices[0].Message.Content)
	}
	if got.Choices[0].FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want stop", got.Choices[0].FinishReason)
	}
}

func TestResponsesToChatResponse_ignoresUnknownContentType(t *testing.T) {
	r := &responsesResponse{
		ID:    "r3",
		Model: "m",
		Output: []responsesOutputItem{
			{Type: "message", Content: []responsesContent{
				{Type: "output_text", Text: "A"},
				{Type: "refusal", Text: "should be ignored"},
				{Type: "output_text", Text: "B"},
			}},
		},
	}
	got := responsesToChatResponse(r)
	if got.Choices[0].Message.Content != "AB" {
		t.Errorf("Content = %q, want AB", got.Choices[0].Message.Content)
	}
}

func TestResponsesChatCompletion_non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	c := NewWithProvider("sk-responses", srv.URL, "", types.ProviderOpenAIResponses)
	req := &types.ChatRequest{
		Model:    "gpt-6-luna",
		Messages: []types.ChatMessage{{Role: "user", Content: "Hi"}},
	}
	_, err := c.ChatCompletion(req)
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %q, should contain 400", err.Error())
	}
}
