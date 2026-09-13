package chat

import (
	"strings"
	"testing"

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
	tools := []string{"generate_image", "generate_video", "midjourney_imagine",
		"midjourney_describe", "midjourney_reroll", "midjourney_video",
		"search_ideas", "balance", "task"}
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

func TestBuildAgentTools_filterImageOnly(t *testing.T) {
	cfg := &types.ChatDefaults{
		Tools: []string{"generate_image"},
	}
	tools := buildAgentTools(cfg)
	if len(tools) != 1 || tools[0].Function.Name != "generate_image" {
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

func TestBuildAgentTools_disableVideo(t *testing.T) {
	cfg := &types.ChatDefaults{
		DisableTools: []string{"generate_video"},
	}
	tools := buildAgentTools(cfg)
	for _, t2 := range tools {
		if t2.Function.Name == "generate_video" {
			t.Error("buildAgentTools should have disabled generate_video")
		}
	}
}
