package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/mcpcli"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func resolveFileRefs(argsJSON string) string {
	var raw interface{}
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return argsJSON // not valid JSON, return as-is
	}
	resolved := resolveFileRef(raw)
	data, _ := json.Marshal(resolved)
	return string(data)
}

// resolveFileRef recursively walks a parsed JSON value and replaces any
// string starting with "@" by reading the referenced file.
func resolveFileRef(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "@") {
			path := val[1:] // strip the @ prefix
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("[Error: cannot read %s: %v]", path, err)
			}
			return string(content)
		}
		return val
	case map[string]interface{}:
		for k, nested := range val {
			val[k] = resolveFileRef(nested)
		}
		return val
	case []interface{}:
		for i, item := range val {
			val[i] = resolveFileRef(item)
		}
		return val
	default:
		return val
	}
}

// executeToolCall executes a single built-in primitive tool call.
func executeToolCall(c *client.Client, tc types.ToolCall) string {
	return executeToolCallMCP(c, nil, tc)
}

// executeToolCallMCP executes a tool call, preferring a connected MCP server
// when it provides the tool, and falling back to the built-in primitive
// dispatcher otherwise. mcp may be nil.
func executeToolCallMCP(c *client.Client, mcp *mcpcli.Manager, tc types.ToolCall) string {
	if mcp.Has(tc.Function.Name) {
		out, err := mcp.Call(context.Background(), tc.Function.Name, tc.Function.Arguments)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return out
	}

	// Expand @filename references in arguments before dispatching.
	// Skip read — it handles its own file access.
	args := tc.Function.Arguments
	if tc.Function.Name != "read" && strings.Contains(args, `"@`) {
		resolved := resolveFileRefs(args)
		if resolved != args {
			if options.Shared.Verbose {
				fmt.Fprintf(options.Stderr(), "\r\n[agent] resolved @file refs in %s\r\n", tc.Function.Name)
			}
			args = resolved
		}
	}

	switch tc.Function.Name {
	case "read":
		return agent.ReadFileWithPolicy(args, readDenyPatterns())
	case "write":
		return agent.WriteFile(args)
	case "edit":
		return agent.EditFile(args)
	case "bash":
		return agent.Bash(args)
	case "grep":
		return agent.Grep(args)
	case "find":
		return agent.FindFiles(args)
	case "ls":
		return agent.ListDir(args)
	case "web_fetch":
		return executeWebFetch(args)
	case "web_search":
		return executeWebSearch(args)
	default:
		return fmt.Sprintf("Error: unknown tool '%s'", tc.Function.Name)
	}
}

// readDenyPatterns returns the configured regex denylist for the read primitive
// (defaults.chat.read_deny). Empty means every file is readable.
func readDenyPatterns() []string {
	if cfg := options.ChatDefaults(); cfg != nil {
		return cfg.ReadDeny
	}
	return nil
}

// summarizeToolResult returns a one-line summary of a tool's result for user display.
func summarizeToolResult(toolName, result string) string {
	// Truncate long results to first meaningful line
	firstLine := result
	if idx := strings.IndexAny(result, "\n\r"); idx > 0 {
		firstLine = result[:idx]
	}
	// Strip common prefixes for cleaner display
	firstLine = strings.TrimSpace(firstLine)
	if strings.HasPrefix(firstLine, "Successfully generated") {
		return firstLine
	}
	if strings.HasPrefix(firstLine, "Error:") {
		return firstLine
	}
	if len(firstLine) > 80 {
		firstLine = firstLine[:80] + "..."
	}
	return firstLine
}
