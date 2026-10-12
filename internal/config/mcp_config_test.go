package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_mcpServers(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yaml")
	content := []byte(`
mcp:
  servers:
    - name: aigc
      command: aigc-cli
      args: ["mcp"]
      tools_disable: ["remove_watermark"]
    - name: off
      command: npx
      enabled: false
`)
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MCP == nil || len(cfg.MCP.Servers) != 2 {
		t.Fatalf("MCP.Servers = %+v, want 2 entries", cfg.MCP)
	}

	s := cfg.MCP.Servers[0]
	if s.Name != "aigc" || s.Command != "aigc-cli" {
		t.Errorf("server[0] name/command = %q/%q", s.Name, s.Command)
	}
	if len(s.Args) != 1 || s.Args[0] != "mcp" {
		t.Errorf("server[0] args = %v, want [mcp]", s.Args)
	}
	if len(s.ToolsDisable) != 1 || s.ToolsDisable[0] != "remove_watermark" {
		t.Errorf("server[0] tools_disable = %v", s.ToolsDisable)
	}
	if !s.IsEnabled() {
		t.Error("server[0] should be enabled by default")
	}
	if cfg.MCP.Servers[1].IsEnabled() {
		t.Error("server[1] should be disabled")
	}
}
