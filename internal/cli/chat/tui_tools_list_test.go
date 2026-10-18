package chat

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func toolDef(name, desc string) types.ToolDefinition {
	return types.ToolDefinition{
		Type:     "function",
		Function: types.ToolFunction{Name: name, Description: desc},
	}
}

func TestCollapsedSummary(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		{"single line", "Read a file.", "Read a file."},
		{"first non-empty line", "\n\nConvert an image.\n\nDetails here.", "Convert an image."},
		{"inner whitespace collapsed", "Convert   an   image.\nmore", "Convert an image."},
		{"empty", "", "(no description)"},
		{"only whitespace", "   \n \t \n", "(no description)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := collapsedSummary(tc.desc, 200); got != tc.want {
				t.Errorf("collapsedSummary() = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("truncates on runes", func(t *testing.T) {
		got := collapsedSummary("日本語テキストです", 4)
		if got != "日本語テ…" {
			t.Errorf("collapsedSummary() = %q, want 4 runes + ellipsis", got)
		}
	})
}

// The list view must keep the one-tool-per-line shape: tool descriptions are
// multi-line schema text (some 24 lines with an Examples block), and inlining
// them verbatim used to push every continuation line to column 0.
func TestShowTools_listKeepsOneLinePerTool(t *testing.T) {
	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.agentTools = []types.ToolDefinition{
		toolDef("convert_depth", "Convert an image or video into a grayscale depth map.\n\nOffline — no API key.\n\nExamples:\n  convert_depth input_path=\"/a.jpg\""),
		toolDef("read", "Read a local text file."),
		toolDef("nodesc", ""),
	}

	m.showTools("")
	content := m.messages[len(m.messages)-1].content
	t.Logf("rendered /tools output:\n%s", content)

	toolLines := 0
	for _, ln := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(ln, "  /"):
			toolLines++
		case strings.TrimSpace(ln) == "":
		case strings.HasPrefix(ln, "Available tools:"):
		case strings.HasPrefix(ln, "Usage:"):
		case strings.HasPrefix(ln, "Full description:"):
		default:
			t.Errorf("non-aligned line in /tools output: %q", ln)
		}
	}
	if toolLines != 3 {
		t.Errorf("listed %d tools, want 3\n%s", toolLines, content)
	}
	if strings.Contains(content, "Examples:") || strings.Contains(content, "Offline — no API key.") {
		t.Errorf("multi-line description leaked into the list view:\n%s", content)
	}
	if !strings.Contains(content, "Convert an image or video into a grayscale depth map.") {
		t.Errorf("summary line missing:\n%s", content)
	}
	if !strings.Contains(content, "(no description)") {
		t.Errorf("empty description should fall back to a placeholder:\n%s", content)
	}
}

func TestShowTools_detailPrintsFullDescription(t *testing.T) {
	newModel := func() chatModel {
		m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
		m.agentTools = []types.ToolDefinition{
			toolDef("convert_depth", "Convert an image or video into a grayscale depth map.\n\nExamples:\n  convert_depth input_path=\"/a.jpg\""),
		}
		return m
	}

	for _, name := range []string{"convert_depth", "/convert_depth"} {
		m := newModel()
		m.showTools(name)
		content := m.messages[len(m.messages)-1].content
		if !strings.Contains(content, "Examples:") || !strings.Contains(content, `input_path="/a.jpg"`) {
			t.Errorf("showTools(%q) dropped the full description:\n%s", name, content)
		}
		if !strings.HasPrefix(content, "/convert_depth") {
			t.Errorf("showTools(%q) should head with the tool name:\n%s", name, content)
		}
	}
}

func TestShowTools_unknownName(t *testing.T) {
	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.agentTools = []types.ToolDefinition{toolDef("read", "Read a file.")}

	m.showTools("nope")
	content := m.messages[len(m.messages)-1].content
	if !strings.Contains(content, `Unknown tool "nope"`) || !strings.Contains(content, "/tools") {
		t.Errorf("unknown tool message = %q, want an Unknown-tool hint", content)
	}
}

func TestShowTools_noTools(t *testing.T) {
	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.agentTools = nil
	m.showTools("")
	if got := m.messages[len(m.messages)-1].content; got != "No tools available." {
		t.Errorf("content = %q, want the empty-state message", got)
	}
}
