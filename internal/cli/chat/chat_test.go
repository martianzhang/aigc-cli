package chat

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// --- Agent Tool Tests ---

func TestToURLs_empty(t *testing.T) {
	got := toURLs("")
	if got != nil {
		t.Errorf("toURLs('') = %v, want nil", got)
	}
}

func TestToURLs_single(t *testing.T) {
	got := toURLs("https://example.com/img.png")
	if len(got) != 1 || got[0] != "https://example.com/img.png" {
		t.Errorf("toURLs() = %v, want [https://example.com/img.png]", got)
	}
}

func TestFormatShellOutput(t *testing.T) {
	got := formatShellOutput("ls -la", "total 0\nfile.txt")
	if !strings.HasPrefix(got, "$ ls -la\n") {
		t.Errorf("command line missing: %q", got)
	}
	if !strings.Contains(got, "file.txt") {
		t.Errorf("shell output missing: %q", got)
	}
}

func TestProviderModelDisplay(t *testing.T) {
	withProvider := &chatModel{model: "deepseek-v4-flash", provider: "apimart"}
	if got := withProvider.providerModelDisplay(); got != "Provider: apimart  |  Model: deepseek-v4-flash" {
		t.Errorf("with provider = %q", got)
	}
	noProvider := &chatModel{model: "m"}
	if got := noProvider.providerModelDisplay(); got != "Model: m" {
		t.Errorf("no provider = %q", got)
	}
}

func TestExecuteToolCall_unknown(t *testing.T) {
	tc := types.ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: types.ToolCallFunction{
			Name:      "nonexistent_tool",
			Arguments: "{}",
		},
	}
	got := executeToolCall(nil, tc)
	if !strings.Contains(got, "unknown tool") {
		t.Errorf("executeToolCall(unknown) = %q, want 'unknown tool'", got)
	}
}

func TestExecuteToolCall_invalidJSON(t *testing.T) {
	tools := []string{"read", "write", "edit", "bash", "grep", "find", "ls", "search", "fetch", "web_fetch", "web_search"}
	for _, name := range tools {
		tc := types.ToolCall{
			ID:   "call_1",
			Type: "function",
			Function: types.ToolCallFunction{
				Name:      name,
				Arguments: "{bad json}",
			},
		}
		got := executeToolCall(nil, tc)
		if !strings.Contains(got, "invalid arguments") {
			t.Errorf("executeToolCall(%s, bad json) = %q, want 'invalid arguments'", name, got)
		}
	}
}

func TestBuildAgentTools_allAllowed(t *testing.T) {
	cfg := &types.ChatDefaults{
		Tools: []string{"*"},
	}
	tools := buildAgentTools(cfg)
	if len(tools) == 0 {
		t.Error("buildAgentTools([\"*\"]) returned empty")
	}
}

func TestBuildAgentTools_disabledAll(t *testing.T) {
	cfg := &types.ChatDefaults{
		DisableTools: []string{"*"},
	}
	tools := buildAgentTools(cfg)
	if len(tools) != 0 {
		t.Errorf("buildAgentTools(disable *) = %d tools, want 0", len(tools))
	}
}

func TestBuildAgentTools_filterSinglePrimitive(t *testing.T) {
	cfg := &types.ChatDefaults{
		Tools: []string{"read"},
	}
	tools := buildAgentTools(cfg)
	if len(tools) != 1 || tools[0].Function.Name != "read" {
		t.Errorf("buildAgentTools filter = got %d tools", len(tools))
	}
}

func TestSummarizeToolResult_truncated(t *testing.T) {
	got := summarizeToolResult("test", "This is a very long result that should definitely be truncated because it exceeds eighty characters in total length")
	if len(got) > 83 {
		t.Errorf("summarizeToolResult() = %q (len=%d), want <= 83", got, len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("summarizeToolResult() = %q, should end with '...'", got)
	}
}

func TestSummarizeToolResult_short(t *testing.T) {
	got := summarizeToolResult("test", "short result")
	if got != "short result" {
		t.Errorf("summarizeToolResult() = %q, want 'short result'", got)
	}
}

func TestSummarizeToolResult_firstLine(t *testing.T) {
	got := summarizeToolResult("test", "first line\nsecond line")
	if got != "first line" {
		t.Errorf("summarizeToolResult() = %q, want 'first line'", got)
	}
}

func TestSummarizeToolResult_successPrefix(t *testing.T) {
	got := summarizeToolResult("generate_image", "Successfully generated image: file.png")
	if got != "Successfully generated image: file.png" {
		t.Errorf("summarizeToolResult() = %q, want full success message", got)
	}
}

func TestSummarizeToolResult_errorPrefix(t *testing.T) {
	got := summarizeToolResult("test", "Error: something went wrong")
	if got != "Error: something went wrong" {
		t.Errorf("summarizeToolResult() = %q, want full error message", got)
	}
}

func TestSummarizeToolResult_empty(t *testing.T) {
	got := summarizeToolResult("test", "")
	if got != "" {
		t.Errorf("summarizeToolResult() = %q, want empty", got)
	}
}

func TestSummarizeToolResult_justNewline(t *testing.T) {
	got := summarizeToolResult("test", "\n")
	if got != "" {
		t.Errorf("summarizeToolResult() = %q, want empty", got)
	}
}

func TestBuildAgentTools_disablePrimitive(t *testing.T) {
	cfg := &types.ChatDefaults{
		DisableTools: []string{"read"},
	}
	tools := buildAgentTools(cfg)
	for _, t2 := range tools {
		if t2.Function.Name == "read" {
			t.Error("buildAgentTools should have disabled read")
		}
	}
}

func TestBuildAgentTools_defaultReadOnly(t *testing.T) {
	prev := options.Shared.Cfg
	options.Shared.Cfg = nil
	t.Cleanup(func() { options.Shared.Cfg = prev })

	tools := buildAgentTools(&types.ChatDefaults{})
	names := make(map[string]bool, len(tools))
	for _, t2 := range tools {
		names[t2.Function.Name] = true
	}
	for _, want := range []string{"read", "grep", "find", "ls"} {
		if !names[want] {
			t.Errorf("read-only primitive %q should be enabled by default", want)
		}
	}
	for _, priv := range []string{"write", "edit", "bash", "search", "fetch", "web_fetch", "web_search"} {
		if names[priv] {
			t.Errorf("privileged primitive %q must be off by default", priv)
		}
	}
}

func TestBuildAgentTools_privilegedRequiresEnable(t *testing.T) {
	prev := options.Shared.Cfg
	options.Shared.Cfg = &types.Config{ToolsEnable: []string{"bash"}}
	t.Cleanup(func() { options.Shared.Cfg = prev })

	tools := buildAgentTools(&types.ChatDefaults{})
	names := make(map[string]bool, len(tools))
	for _, t2 := range tools {
		names[t2.Function.Name] = true
	}
	if !names["bash"] {
		t.Error("bash should be enabled when listed in tools_enable")
	}
	if names["write"] {
		t.Error("write should stay disabled when not listed")
	}
	if !names["read"] {
		t.Error("read-only primitive should remain enabled")
	}
}

func toolNames(tools []types.ToolDefinition) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		names[t.Function.Name] = true
	}
	return names
}

func TestToolRuntime_includesMCPByDefault(t *testing.T) {
	prev := options.Shared.Cfg
	options.Shared.Cfg = nil
	t.Cleanup(func() { options.Shared.Cfg = prev })

	rt := newToolRuntime(&types.ChatDefaults{}, nil)
	names := toolNames(rt.RequestTools())
	if !names["read"] {
		t.Error("read primitive missing")
	}
}

func TestToolRuntime_onDemandHidesMCPUntilLoaded(t *testing.T) {
	prev := options.Shared.Cfg
	options.Shared.Cfg = nil
	t.Cleanup(func() { options.Shared.Cfg = prev })

	rt := newToolRuntime(&types.ChatDefaults{ToolsOnDemand: true}, nil)
	// Inject a fake MCP tool directly (no server needed).
	rt.mcpTools = []types.ToolDefinition{{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "generate_image",
			Description: "Generate images via AI",
		},
	}}

	names := toolNames(rt.RequestTools())
	if names["generate_image"] {
		t.Fatal("MCP tool should be hidden before load_tools")
	}
	if !names[searchToolsName] || !names[loadToolsName] {
		t.Fatal("meta-tools should be exposed in on-demand mode")
	}

	if got := rt.Execute(nil, types.ToolCall{Function: types.ToolCallFunction{Name: loadToolsName, Arguments: `{"names":["generate_image"]}`}}); !strings.Contains(got, "Loaded") {
		t.Fatalf("load_tools = %q", got)
	}
	if !toolNames(rt.RequestTools())["generate_image"] {
		t.Error("MCP tool should be visible after load_tools")
	}

	if got := rt.Execute(nil, types.ToolCall{Function: types.ToolCallFunction{Name: searchToolsName, Arguments: `{"query":"image"}`}}); !strings.Contains(got, "generate_image") {
		t.Errorf("search_tools = %q", got)
	}
}
