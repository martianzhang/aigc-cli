package mcpcli

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestConvertTool(t *testing.T) {
	tool := mcp.Tool{
		Name:        "generate_image",
		Description: "Generate an image",
		InputSchema: mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{"prompt": map[string]any{"type": "string"}},
			Required:   []string{"prompt"},
		},
	}

	got, err := convertTool(tool)
	if err != nil {
		t.Fatalf("convertTool: %v", err)
	}
	if got.Type != "function" {
		t.Errorf("Type = %q, want function", got.Type)
	}
	if got.Function.Name != "generate_image" {
		t.Errorf("Name = %q, want generate_image", got.Function.Name)
	}
	if got.Function.Description != "Generate an image" {
		t.Errorf("Description = %q", got.Function.Description)
	}

	var schema map[string]any
	if err := json.Unmarshal(got.Function.Parameters, &schema); err != nil {
		t.Fatalf("parameters not valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}
}

func TestResultText(t *testing.T) {
	tests := []struct {
		name string
		res  *mcp.CallToolResult
		want string
	}{
		{"nil", nil, ""},
		{"text only", &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Text: "hello"}},
		}, "hello"},
		{"multiple text", &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Text: "a"},
				mcp.TextContent{Text: "b"},
			},
		}, "a\nb"},
		{"image ignored when text present", &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Text: "已保存的图片:\n  /out/a.png"},
				mcp.ImageContent{Data: "x", MIMEType: "image/png"},
			},
		}, "已保存的图片:\n  /out/a.png"},
		{"media only", &mcp.CallToolResult{
			Content: []mcp.Content{mcp.ImageContent{Data: "x", MIMEType: "image/png"}},
		}, "[media result without a local file path]"},
		{"error with no content", &mcp.CallToolResult{IsError: true}, "tool returned an error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resultText(tt.res); got != tt.want {
				t.Errorf("resultText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToolAllowed(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		enable  []string
		disable []string
		want    bool
	}{
		{"empty allows all", "generate_image", nil, nil, true},
		{"enable match", "generate_image", []string{"generate_*"}, nil, true},
		{"enable no match", "upscale", []string{"generate_*"}, nil, false},
		{"disable wins", "upscale", []string{"*"}, []string{"upscale"}, false},
		{"disable only", "upscale", nil, []string{"upscale"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolAllowed(tt.tool, tt.enable, tt.disable); got != tt.want {
				t.Errorf("toolAllowed(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func TestServerName(t *testing.T) {
	if got := serverName(types.MCPServer{Name: "aigc", Command: "aigc-cli"}); got != "aigc" {
		t.Errorf("named = %q, want aigc", got)
	}
	if got := serverName(types.MCPServer{Command: "npx"}); got != "npx" {
		t.Errorf("fallback = %q, want npx", got)
	}
}
