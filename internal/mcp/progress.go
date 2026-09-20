package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Coarse progress milestones for the async generation tools (video / music).
// Only these two are emitted on purpose — no fake precision, because per-poll
// percentages would need hooks inside the CLI dispatch loops.
const (
	// progressDispatch is sent right before the blocking dispatch call.
	progressDispatch = 0.05
	// progressGenerating is sent right before the long submit → poll phase.
	progressGenerating = 0.40
)

// progressGeneratingMessage explains why the indicator parks at 0.40.
const progressGeneratingMessage = "generation in progress (submit → poll phase can take minutes; no per-poll updates yet)"

// progressReporter emits coarse-phase MCP progress notifications for one tool
// call. When the host did not send a progressToken, or no MCP session is
// available in ctx, it is a silent no-op (zero cost, zero log noise).
//
// A tool call runs synchronously in its handler goroutine, so the reporter
// carries no lock and is safe to keep value-only.
type progressReporter struct {
	token mcp.ProgressToken
	emit  func(mcp.ProgressNotification)
}

// newProgressReporter builds a reporter from the tool request and the session
// stored in ctx by the MCP server. The emit dependency is captured at
// construction so tests can drive the reporter without any transport.
func newProgressReporter(ctx context.Context, request mcp.CallToolRequest) *progressReporter {
	token := progressToken(request)
	if token == nil {
		return &progressReporter{}
	}
	session := server.ClientSessionFromContext(ctx)
	if session == nil {
		return &progressReporter{}
	}
	channel := session.NotificationChannel()
	return &progressReporter{
		token: token,
		emit: func(notification mcp.ProgressNotification) {
			// Non-blocking send: a full or unread channel must never stall or
			// fail the tool call.
			select {
			case channel <- progressToJSONRPC(notification):
			default:
			}
		},
	}
}

// report emits one progress notification. It is deliberately lossy: a reporter
// without a token or emit function does nothing, so callers never branch.
func (p *progressReporter) report(progress float64, message string) {
	if p == nil || p.token == nil || p.emit == nil {
		return
	}
	p.emit(mcp.NewProgressNotification(p.token, progress, nil, &message))
}

// progressToken extracts request.Params.Meta.ProgressToken, nil-safe.
func progressToken(request mcp.CallToolRequest) mcp.ProgressToken {
	if request.Params.Meta == nil {
		return nil
	}
	return request.Params.Meta.ProgressToken
}

// progressDispatchMessage renders the 0.05 milestone text.
func progressDispatchMessage(providerType string) string {
	return "dispatching to " + providerType + " …"
}

// progressToJSONRPC converts a ProgressNotification into the JSON-RPC envelope
// the session channel carries. ProgressNotification.Params (typed) shadows the
// embedded Notification.Params, so the fields are copied into AdditionalFields
// which NotificationParams marshals inline.
func progressToJSONRPC(notification mcp.ProgressNotification) mcp.JSONRPCNotification {
	fields := map[string]any{
		"progressToken": notification.Params.ProgressToken,
		"progress":      notification.Params.Progress,
	}
	if notification.Params.Total != 0 {
		fields["total"] = notification.Params.Total
	}
	if notification.Params.Message != "" {
		fields["message"] = notification.Params.Message
	}
	return mcp.JSONRPCNotification{
		JSONRPC: mcp.JSONRPC_VERSION,
		Notification: mcp.Notification{
			Method: notification.Method,
			Params: mcp.NotificationParams{AdditionalFields: fields},
		},
	}
}
