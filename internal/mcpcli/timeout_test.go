package mcpcli

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestMCPInitTimeoutFallbackAndClamp(t *testing.T) {
	prev := mcpInitTimeout()
	t.Cleanup(func() { SetInitTimeout(prev) })

	SetInitTimeout(0)
	if got := mcpInitTimeout(); got != DefaultInitTimeout {
		t.Errorf("mcpInitTimeout() = %v, want built-in %v", got, DefaultInitTimeout)
	}

	SetInitTimeout(5 * time.Second)
	if got := mcpInitTimeout(); got != 5*time.Second {
		t.Errorf("mcpInitTimeout() = %v, want 5s", got)
	}

	SetInitTimeout(time.Hour)
	if got := mcpInitTimeout(); got != MaxInitTimeout {
		t.Errorf("mcpInitTimeout() = %v, want cap %v", got, MaxInitTimeout)
	}
}

// TestNewManagerTimesOutHungServer guards that a server that never answers the
// handshake cannot block chat startup indefinitely.
func TestNewManagerTimesOutHungServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep is not available on windows")
	}
	prev := mcpInitTimeout()
	t.Cleanup(func() { SetInitTimeout(prev) })
	SetInitTimeout(100 * time.Millisecond)

	cfg := &types.MCPConfig{Servers: []types.MCPServer{{
		Name:    "hung",
		Command: "sleep",
		Args:    []string{"30"},
	}}}

	start := time.Now()
	m, err := NewManager(context.Background(), cfg)
	if m != nil {
		defer m.Close()
	}
	if err == nil {
		t.Fatal("NewManager() error = nil, want a startup timeout")
	}
	if m != nil && len(m.Tools()) != 0 {
		t.Errorf("tools = %d, want 0 from a server that never initialized", len(m.Tools()))
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("NewManager took %v, want it to honor the 100ms init timeout", elapsed)
	}
}
