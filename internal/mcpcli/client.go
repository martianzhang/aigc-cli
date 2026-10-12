// Package mcpcli is a thin MCP (Model Context Protocol) client used by the chat
// agent to consume tools from user-configured servers (see `mcp.servers` in
// config.yaml). There is no built-in server: users point at `aigc-cli mcp` or
// any third-party MCP server explicitly.
package mcpcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// clientVersion is reported to servers during the MCP handshake.
const clientVersion = "0.1.0"

// Server is a connected MCP server together with the tools it exposes.
type Server struct {
	Name   string
	client *client.Client
	tools  []types.ToolDefinition
}

// Tools returns the (filtered) tool definitions exposed by this server.
func (s *Server) Tools() []types.ToolDefinition { return s.tools }

// Close shuts down the underlying client and its transport.
func (s *Server) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

// Call invokes a tool on this server and returns its text result. argsJSON is
// the raw JSON object the model produced (may be empty for no-argument tools).
func (s *Server) Call(ctx context.Context, name, argsJSON string) (string, error) {
	var args map[string]any
	if trimmed := strings.TrimSpace(argsJSON); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	res, err := s.client.CallTool(ctx, req)
	if err != nil {
		return "", err
	}
	return resultText(res), nil
}

// ConnectAll launches every enabled server in cfg and returns the connected set.
// Servers that fail to start are skipped; their errors are collected and joined,
// so one bad entry does not disable the others.
func ConnectAll(ctx context.Context, cfg *types.MCPConfig) ([]*Server, error) {
	if cfg == nil {
		return nil, nil
	}

	var servers []*Server
	var errs []error
	for _, sc := range cfg.Servers {
		if !sc.IsEnabled() {
			continue
		}
		s, err := connect(ctx, sc)
		if err != nil {
			errs = append(errs, fmt.Errorf("mcp server %q: %w", serverName(sc), err))
			continue
		}
		servers = append(servers, s)
	}
	return servers, errors.Join(errs...)
}

func connect(ctx context.Context, sc types.MCPServer) (*Server, error) {
	if sc.Command == "" {
		return nil, errors.New("command is required")
	}

	env := append(os.Environ(), sc.Env...)
	c, err := client.NewStdioMCPClient(sc.Command, env, sc.Args...)
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	if _, err := c.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "aigc-cli", Version: clientVersion},
		},
	}); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	res, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("list tools: %w", err)
	}

	tools := make([]types.ToolDefinition, 0, len(res.Tools))
	for _, t := range res.Tools {
		if !toolAllowed(t.Name, sc.ToolsEnable, sc.ToolsDisable) {
			continue
		}
		td, err := convertTool(t)
		if err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("convert tool %q: %w", t.Name, err)
		}
		tools = append(tools, td)
	}

	return &Server{Name: serverName(sc), client: c, tools: tools}, nil
}

func serverName(sc types.MCPServer) string {
	if sc.Name != "" {
		return sc.Name
	}
	return sc.Command
}

// toolAllowed applies the per-server glob allow/deny lists. Disable wins.
func toolAllowed(name string, enable, disable []string) bool {
	if len(enable) > 0 && !matchAny(name, enable) {
		return false
	}
	return !matchAny(name, disable)
}

func matchAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}
