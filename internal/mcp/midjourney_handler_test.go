package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// toolResultText returns the first text content of a tool result.
func toolResultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("expected non-nil tool result")
	}
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatalf("no text content in result: %+v", res.Content)
	return ""
}

// ----- toURLs -----

func TestToURLs_empty(t *testing.T) {
	if got := toURLs(""); got != nil {
		t.Errorf("toURLs(\"\") = %v, want nil", got)
	}
}

func TestToURLs_single(t *testing.T) {
	got := toURLs("https://example.com/img.png")
	if len(got) != 1 || got[0] != "https://example.com/img.png" {
		t.Errorf("toURLs() = %v", got)
	}
}

// ----- handlers (argument validation only; no network) -----

func TestMidjourneyDescribeHandler_missingImageURL(t *testing.T) {
	cfg := &Config{APIKey: "sk-test", BaseURL: "https://api.example.com/v1"}
	req := mcp.CallToolRequest{}
	req.Params.Name = "midjourney_describe"
	req.Params.Arguments = map[string]any{}

	res, err := midjourneyDescribeHandler(cfg)(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for missing image_url")
	}
	if got := toolResultText(t, res); got != "image_url is required" {
		t.Errorf("message = %q", got)
	}
}

func TestMidjourneyVideoHandler_missingImageURL(t *testing.T) {
	cfg := &Config{APIKey: "sk-test", BaseURL: "https://api.example.com/v1"}
	req := mcp.CallToolRequest{}
	req.Params.Name = "midjourney_video"
	req.Params.Arguments = map[string]any{}

	res, err := midjourneyVideoHandler(cfg)(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for missing image_url")
	}
	if got := toolResultText(t, res); got != "image_url is required" {
		t.Errorf("message = %q", got)
	}
}

func TestMidjourneyRerollHandler_missingTaskID(t *testing.T) {
	cfg := &Config{APIKey: "sk-test", BaseURL: "https://api.example.com/v1"}
	req := mcp.CallToolRequest{}
	req.Params.Name = "midjourney_reroll"
	req.Params.Arguments = map[string]any{}

	res, err := midjourneyRerollHandler(cfg)(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for missing task_id")
	}
	if got := toolResultText(t, res); got != "task_id is required" {
		t.Errorf("message = %q", got)
	}
}

func TestMidjourneyImagineHandler_missingPrompt(t *testing.T) {
	cfg := &Config{APIKey: "sk-test", BaseURL: "https://api.example.com/v1"}
	req := mcp.CallToolRequest{}
	req.Params.Name = "midjourney_imagine"
	req.Params.Arguments = map[string]any{}

	res, err := midjourneyImagineHandler(cfg)(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError=true for missing prompt")
	}
	if got := toolResultText(t, res); got != "prompt is required" {
		t.Errorf("message = %q", got)
	}
}

func TestMidjourneyHandlers_missingAPIKey(t *testing.T) {
	cfg := &Config{BaseURL: "https://api.example.com/v1"}
	req := mcp.CallToolRequest{}
	req.Params.Name = "midjourney_describe"
	req.Params.Arguments = map[string]any{"image_url": "https://example.com/img.png"}

	for _, handler := range []struct {
		name string
		fn   func(*Config) server.ToolHandlerFunc
	}{
		{"imagine", midjourneyImagineHandler},
		{"describe", midjourneyDescribeHandler},
		{"reroll", midjourneyRerollHandler},
		{"video", midjourneyVideoHandler},
	} {
		res, err := handler.fn(cfg)(context.Background(), req)
		if err != nil {
			t.Fatalf("%s: handler returned transport error: %v", handler.name, err)
		}
		if !res.IsError {
			t.Errorf("%s: expected IsError=true when API key is missing", handler.name)
		}
		if got := toolResultText(t, res); got != "API Key not configured" {
			t.Errorf("%s: message = %q", handler.name, got)
		}
	}
}
