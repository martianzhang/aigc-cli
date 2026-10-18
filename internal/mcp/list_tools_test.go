package mcp

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureListTools runs ListTools with stdout redirected and returns its output.
func captureListTools(t *testing.T, cfg *Config, mode string) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w

	ListTools(cfg, mode)

	os.Stdout = origStdout
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read pipe: %v", err)
	}
	r.Close()
	return string(out)
}

func TestListTools_allShowsEveryToolWithState(t *testing.T) {
	cfg := &Config{ToolsDisable: []string{"get_balance"}}
	out := captureListTools(t, cfg, "all")

	if !strings.Contains(out, "MCP tools: "+itoa(len(toolRegistry))+" registered, "+itoa(len(toolRegistry)-1)+" enabled") {
		t.Errorf("header = %q, want registry/enabled counts", strings.SplitN(out, "\n", 2)[0])
	}

	// Every registered tool must appear, whether enabled or not.
	for _, info := range toolRegistry {
		if !strings.Contains(out, info.name) {
			t.Errorf("output missing registered tool %q", info.name)
		}
		if !strings.Contains(out, info.desc) {
			t.Errorf("output missing description for %q", info.name)
		}
	}

	// The disabled tool is marked and explains which rule blocked it.
	disabledLine := lineContaining(out, "get_balance")
	if !strings.Contains(disabledLine, "⛔") {
		t.Errorf("get_balance line = %q, want a disabled marker", disabledLine)
	}
	if !strings.Contains(disabledLine, "[in tools_disable]") {
		t.Errorf("get_balance line = %q, want the tools_disable reason", disabledLine)
	}

	// An untouched tool stays enabled.
	if line := lineContaining(out, "generate_image"); !strings.Contains(line, "✅") {
		t.Errorf("generate_image line = %q, want an enabled marker", line)
	}

	if !strings.Contains(out, "tools_disable: [get_balance]") {
		t.Error("output should echo the active tools_disable list")
	}
}

func TestListTools_notInEnableListReason(t *testing.T) {
	cfg := &Config{ToolsEnable: []string{"generate_image", "list_models"}}
	out := captureListTools(t, cfg, "all")

	if line := lineContaining(out, "generate_image"); !strings.Contains(line, "✅") {
		t.Errorf("generate_image line = %q, want enabled", line)
	}
	line := lineContaining(out, "generate_video")
	if !strings.Contains(line, "⛔") || !strings.Contains(line, "[not in tools_enable]") {
		t.Errorf("generate_video line = %q, want the tools_enable reason", line)
	}
}

func TestListTools_enabledModeHidesDisabled(t *testing.T) {
	cfg := &Config{ToolsDisable: []string{"get_balance"}}
	out := captureListTools(t, cfg, "enabled")

	if strings.Contains(out, "Query API key or account balance") {
		t.Error("enabled mode should not list the disabled tool")
	}
	if strings.Contains(out, "⛔") {
		t.Error("enabled mode should not print disabled markers")
	}
	if !strings.Contains(out, itoa(len(toolRegistry))+" registered") {
		t.Errorf("header should still mention the registry size: %q", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(out, "generate_image") {
		t.Error("enabled mode should still list enabled tools")
	}
}

func TestListTools_noRulesEnablesEverything(t *testing.T) {
	out := captureListTools(t, &Config{}, "all")
	if strings.Contains(out, "⛔") {
		t.Error("with no enable/disable rules every tool should be enabled")
	}
	if !strings.Contains(out, itoa(len(toolRegistry))+" registered, "+itoa(len(toolRegistry))+" enabled") {
		t.Errorf("header = %q, want all tools enabled", strings.SplitN(out, "\n", 2)[0])
	}
}

// lineContaining returns the first output line containing needle.
func lineContaining(out, needle string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
