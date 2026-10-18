package chat

import (
	"strings"
	"testing"

	mcpserver "github.com/martianzhang/aigc-cli/internal/mcp"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// TestShowTools_realRegistryStaysAligned cross-checks the list view against the
// real MCP tool registry, which is the exact data that used to break it: ten of
// its tools ship multi-line descriptions (convert_depth alone is 24 lines, with
// its own indentation and an Examples block), so the list must stay one line per
// tool regardless of what the descriptions contain.
func TestShowTools_realRegistryStaysAligned(t *testing.T) {
	srv := mcpserver.NewServer(&mcpserver.Config{
		BaseURL:  "https://api.openai.com/v1",
		Defaults: &types.ConfigDefaults{},
	})

	registered := srv.ListTools()
	if len(registered) < 30 {
		t.Fatalf("MCP registry has %d tools, expected the full set", len(registered))
	}

	tools := make([]types.ToolDefinition, 0, len(registered))
	multiLine := 0
	for name, st := range registered {
		desc := st.Tool.Description
		if strings.Contains(desc, "\n") {
			multiLine++
		}
		if desc == "" {
			t.Errorf("tool %q has an empty description", name)
		}
		tools = append(tools, toolDef(name, desc))
	}
	t.Logf("cross-checking %d MCP tools, %d with multi-line descriptions", len(tools), multiLine)
	if multiLine == 0 {
		t.Fatal("expected the registry to contain multi-line descriptions; test would be vacuous")
	}

	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.agentTools = tools
	m.showTools("")
	content := m.messages[len(m.messages)-1].content

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
			t.Errorf("misaligned line in /tools output: %q", ln)
		}
	}
	if toolLines != len(tools) {
		t.Errorf("listed %d tool lines for %d tools", toolLines, len(tools))
	}
	// No continuation text from any description may leak into the list.
	for marker := range map[string]bool{"Examples:": true, "Prerequisite:": true, "Input type": true} {
		if strings.Contains(content, marker) {
			t.Errorf("multi-line description fragment %q leaked into the list view", marker)
		}
	}
}

// TestShowTools_realRegistryDetailKeepsExamples pins that the full text is still
// reachable for the tools whose descriptions carry usage examples.
func TestShowTools_realRegistryDetailKeepsExamples(t *testing.T) {
	srv := mcpserver.NewServer(&mcpserver.Config{
		BaseURL:  "https://api.openai.com/v1",
		Defaults: &types.ConfigDefaults{},
	})
	st, ok := srv.ListTools()["convert_depth"]
	if !ok {
		t.Fatal("convert_depth missing from the MCP registry")
	}

	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.agentTools = []types.ToolDefinition{toolDef("convert_depth", st.Tool.Description)}
	m.showTools("convert_depth")

	content := m.messages[len(m.messages)-1].content
	if !strings.Contains(content, "Examples:") {
		t.Errorf("/tools convert_depth should include the Examples block:\n%s", content)
	}
	// The summary in the list view is a strict prefix of the detail view.
	summary := collapsedSummary(st.Tool.Description, 90)
	if !strings.Contains(content, strings.TrimSuffix(summary, "…")) {
		t.Errorf("detail view should contain the list summary %q", summary)
	}
	if n := strings.Count(content, "\n"); n < 10 {
		t.Errorf("detail view has %d lines, expected the full multi-line description", n)
	}
}
