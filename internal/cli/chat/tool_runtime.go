package chat

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/mcpcli"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// toolRuntime owns the effective tool set for one chat session. It always
// exposes the built-in primitives; MCP tools are either included in full or
// loaded on demand via the search_tools / load_tools meta-tools.
type toolRuntime struct {
	mcp       *mcpcli.Manager
	base      []types.ToolDefinition // primitives, already filtered
	mcpTools  []types.ToolDefinition // MCP tools, already filtered
	onDemand  bool
	activated map[string]bool
}

// newToolRuntime builds the runtime from the chat config and connected servers.
func newToolRuntime(cfg *types.ChatDefaults, mcp *mcpcli.Manager) *toolRuntime {
	rt := &toolRuntime{
		mcp:       mcp,
		base:      filterTools(agent.ToolDefs, cfg),
		mcpTools:  filterTools(mcp.Tools(), cfg),
		activated: map[string]bool{},
	}
	if cfg != nil {
		rt.onDemand = cfg.ToolsOnDemand
	}
	return rt
}

// RequestTools returns the tool definitions to send for the current turn.
//
// Built-in primitives win on name collisions: an MCP server that exposes
// `search`/`fetch`/`read`/… cannot shadow the CLI's own implementation, and the
// request never carries two definitions with the same function name (which most
// providers reject). Collisions are reported on stderr in verbose mode.
func (rt *toolRuntime) RequestTools() []types.ToolDefinition {
	if rt == nil {
		return nil
	}
	out := make([]types.ToolDefinition, 0, len(rt.base)+len(rt.mcpTools)+2)
	seen := make(map[string]bool, len(rt.base)+len(rt.mcpTools)+2)
	for _, t := range rt.base {
		out = append(out, t)
		seen[t.Function.Name] = true
	}

	var shadowed []string
	appendMCP := func(t types.ToolDefinition) {
		if seen[t.Function.Name] {
			shadowed = append(shadowed, t.Function.Name)
			return
		}
		seen[t.Function.Name] = true
		out = append(out, t)
	}

	if !rt.onDemand {
		for _, t := range rt.mcpTools {
			appendMCP(t)
		}
	} else if len(rt.mcpTools) > 0 {
		// On-demand: expose the meta-tools plus only the already-activated MCP tools.
		out = append(out, searchToolsDef, loadToolsDef)
		seen[searchToolsName] = true
		seen[loadToolsName] = true
		for _, t := range rt.mcpTools {
			if rt.activated[t.Function.Name] {
				appendMCP(t)
			}
		}
	}

	if len(shadowed) > 0 && options.Shared.Verbose {
		fmt.Fprintf(options.Stderr(),
			"[tools] %d MCP tool(s) shadowed by built-in primitives and not sent: %s\r\n",
			len(shadowed), strings.Join(shadowed, ", "))
	}
	return out
}

// Execute runs a single tool call, handling the on-demand meta-tools first.
func (rt *toolRuntime) Execute(c *client.Client, tc types.ToolCall) string {
	if rt == nil {
		return executeToolCallMCP(c, nil, tc)
	}
	switch tc.Function.Name {
	case searchToolsName:
		return rt.searchTools(tc.Function.Arguments)
	case loadToolsName:
		return rt.loadTools(tc.Function.Arguments)
	}
	return executeToolCallMCP(c, rt.mcp, tc)
}

const (
	searchToolsName = "search_tools"
	loadToolsName   = "load_tools"
)

var searchToolsDef = types.ToolDefinition{
	Type: "function",
	Function: types.ToolFunction{
		Name:        searchToolsName,
		Description: "Search the available external (MCP) tools by keyword and return their names with short descriptions. Call load_tools with the names you need to make them callable.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "Keyword matched against tool names and descriptions (empty = list all)"}
			}
		}`),
	},
}

var loadToolsDef = types.ToolDefinition{
	Type: "function",
	Function: types.ToolFunction{
		Name:        loadToolsName,
		Description: "Load one or more external (MCP) tools by name so they become callable in subsequent turns. Use names returned by search_tools.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"names": {"type": "array", "items": {"type": "string"}, "description": "Tool names to load"}
			},
			"required": ["names"]
		}`),
	},
}

func (rt *toolRuntime) searchTools(argsJSON string) string {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	q := strings.ToLower(strings.TrimSpace(args.Query))

	var lines []string
	for _, t := range rt.mcpTools {
		name := t.Function.Name
		desc := t.Function.Description
		if q == "" || strings.Contains(strings.ToLower(name), q) || strings.Contains(strings.ToLower(desc), q) {
			lines = append(lines, fmt.Sprintf("- %s: %s", name, shortDescription(desc)))
		}
	}
	if len(lines) == 0 {
		return fmt.Sprintf("No tools match %q.", args.Query)
	}
	sort.Strings(lines)
	return fmt.Sprintf("Matching tools (call load_tools with the names to enable):\n%s", strings.Join(lines, "\n"))
}

func (rt *toolRuntime) loadTools(argsJSON string) string {
	var args struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if len(args.Names) == 0 {
		return "Error: names is required"
	}

	available := make(map[string]bool, len(rt.mcpTools))
	for _, t := range rt.mcpTools {
		available[t.Function.Name] = true
	}

	var loaded, unknown []string
	for _, n := range args.Names {
		switch {
		case available[n]:
			rt.activated[n] = true
			loaded = append(loaded, n)
		default:
			unknown = append(unknown, n)
		}
	}

	var b strings.Builder
	if len(loaded) > 0 {
		fmt.Fprintf(&b, "Loaded tools: %s. They are now callable.", strings.Join(loaded, ", "))
	}
	if len(unknown) > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "Unknown tools (use search_tools to find valid names): %s.", strings.Join(unknown, ", "))
	}
	return b.String()
}

// shortDescription returns the first line of a description, truncated.
func shortDescription(desc string) string {
	line := desc
	if idx := strings.IndexAny(desc, "\n\r"); idx > 0 {
		line = desc[:idx]
	}
	line = strings.TrimSpace(line)
	if len(line) > 100 {
		line = line[:100] + "..."
	}
	return line
}
