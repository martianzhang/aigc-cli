package mcpcli

import (
	"context"
	"os/exec"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// TestNewManager_AigcCLI verifies the stdio handshake against aigc-cli's own
// MCP server. Skipped when the binary is not on $PATH (e.g. CI).
func TestNewManager_AigcCLI(t *testing.T) {
	if _, err := exec.LookPath("aigc-cli"); err != nil {
		t.Skip("aigc-cli not on $PATH")
	}

	cfg := &types.MCPConfig{Servers: []types.MCPServer{{
		Name:    "aigc",
		Command: "aigc-cli",
		Args:    []string{"mcp"},
	}}}

	m, err := NewManager(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Close()

	if len(m.Tools()) == 0 {
		t.Error("expected at least one tool from `aigc-cli mcp`")
	}
}
