package mcpcli

import (
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// convertTool maps an MCP tool definition onto the chat agent's ToolDefinition.
func convertTool(t mcp.Tool) (types.ToolDefinition, error) {
	params, err := json.Marshal(t.InputSchema)
	if err != nil {
		return types.ToolDefinition{}, err
	}
	return types.ToolDefinition{
		Type: "function",
		Function: types.ToolFunction{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  json.RawMessage(params),
		},
	}, nil
}

// resultText flattens an MCP tool result into a single text string for the model.
//
// Media blocks (images/audio) are intentionally NOT inlined into the
// conversation: the tools that produce files already report their local storage
// paths in their text content, and the local path is what the model needs. A
// media-only result with no accompanying text is noted so it is not silently
// dropped.
func resultText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}

	var text []string
	nonText := 0
	for _, c := range res.Content {
		if txt, ok := c.(mcp.TextContent); ok {
			if txt.Text != "" {
				text = append(text, txt.Text)
			}
			continue
		}
		nonText++
	}

	if len(text) > 0 {
		return strings.Join(text, "\n")
	}
	if res.IsError {
		return "tool returned an error"
	}
	if nonText > 0 {
		return "[media result without a local file path]"
	}
	return ""
}
