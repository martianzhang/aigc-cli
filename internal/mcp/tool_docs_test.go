package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestToolDocsListEveryTool keeps the hand-written MCP tool tables in sync with
// the registry. Adding/removing a tool without updating both guides fails here.
func TestToolDocsListEveryTool(t *testing.T) {
	files := []string{
		filepath.Join("..", "..", "docs", "zh", "guide-mcp.md"),
		filepath.Join("..", "..", "docs", "en", "guide-mcp.md"),
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, info := range toolRegistry {
			if !strings.Contains(text, info.name) {
				t.Errorf("%s does not list tool %q", path, info.name)
			}
		}
	}
}
