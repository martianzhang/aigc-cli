package mcpcli

import (
	"context"
	"errors"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// Manager owns a set of connected MCP servers and dispatches tool calls by name.
// The zero value (and a nil *Manager) is safe to use and exposes no tools.
type Manager struct {
	servers []*Server
	tools   []types.ToolDefinition
	index   map[string]*Server
}

// NewManager connects every enabled server in cfg and indexes their tools.
// Servers that fail to connect are skipped; the returned error (if any) reports
// which ones failed so callers can surface it without aborting the session.
func NewManager(ctx context.Context, cfg *types.MCPConfig) (*Manager, error) {
	servers, err := ConnectAll(ctx, cfg)

	m := &Manager{servers: servers, index: map[string]*Server{}}
	for _, s := range servers {
		for _, t := range s.tools {
			if _, exists := m.index[t.Function.Name]; exists {
				continue // first server wins on duplicate tool names
			}
			m.index[t.Function.Name] = s
			m.tools = append(m.tools, t)
		}
	}
	return m, err
}

// Tools returns the union of tools across connected servers, in server order.
func (m *Manager) Tools() []types.ToolDefinition {
	if m == nil {
		return nil
	}
	return m.tools
}

// Has reports whether name is provided by any connected server.
func (m *Manager) Has(name string) bool {
	if m == nil {
		return false
	}
	_, ok := m.index[name]
	return ok
}

// Call invokes a tool by name on the owning server.
func (m *Manager) Call(ctx context.Context, name, argsJSON string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("unknown mcp tool %q", name)
	}
	s, ok := m.index[name]
	if !ok {
		return "", fmt.Errorf("unknown mcp tool %q", name)
	}
	return s.Call(ctx, name, argsJSON)
}

// Close shuts down all connected servers.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	var errs []error
	for _, s := range m.servers {
		if err := s.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
