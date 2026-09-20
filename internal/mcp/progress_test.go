package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// fakeClientSession is a transport-free server.ClientSession backed by a channel.
type fakeClientSession struct {
	ch chan mcp.JSONRPCNotification
}

func (f *fakeClientSession) Initialize()       {}
func (f *fakeClientSession) Initialized() bool { return true }
func (f *fakeClientSession) SessionID() string { return "progress-test-session" }
func (f *fakeClientSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return f.ch
}

// sessionContext injects a fake session through the SDK's own context key.
func sessionContext(ch chan mcp.JSONRPCNotification) context.Context {
	return server.NewMCPServer("progress-test", "0.0.0").
		WithContext(context.Background(), &fakeClientSession{ch: ch})
}

func requestWithToken(token mcp.ProgressToken) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{Meta: &mcp.Meta{ProgressToken: token}},
	}
}

func Test_progressReporter_emitsCoarseMilestones_whenTokenAndSessionPresent(t *testing.T) {
	// Given: a request carrying a progressToken and a session listening in ctx
	notifications := make(chan mcp.JSONRPCNotification, 4)
	reporter := newProgressReporter(sessionContext(notifications), requestWithToken("tok-1"))

	// When: the reporter emits the two handler milestones
	reporter.report(progressDispatch, progressDispatchMessage("OpenRouter"))
	reporter.report(progressGenerating, progressGeneratingMessage)

	// Then: two notifications arrive, each with the exact wire-level values
	want := []struct {
		progress float64
		message  string
	}{
		{0.05, "dispatching to OpenRouter …"},
		{0.40, "generation in progress (submit → poll phase can take minutes; no per-poll updates yet)"},
	}
	if got := len(notifications); got != len(want) {
		t.Fatalf("got %d notifications, want %d", got, len(want))
	}
	for i, w := range want {
		raw, err := json.Marshal(<-notifications)
		if err != nil {
			t.Fatalf("marshal notification %d: %v", i, err)
		}
		var decoded struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				ProgressToken any     `json:"progressToken"`
				Progress      float64 `json:"progress"`
				Message       string  `json:"message"`
			} `json:"params"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("unmarshal notification %d: %v", i, err)
		}
		if decoded.JSONRPC != mcp.JSONRPC_VERSION {
			t.Errorf("[%d] jsonrpc = %q, want %q", i, decoded.JSONRPC, mcp.JSONRPC_VERSION)
		}
		if decoded.Method != string(mcp.MethodNotificationProgress) {
			t.Errorf("[%d] method = %q, want %q", i, decoded.Method, mcp.MethodNotificationProgress)
		}
		if decoded.Params.ProgressToken != "tok-1" {
			t.Errorf("[%d] progressToken = %v, want tok-1", i, decoded.Params.ProgressToken)
		}
		if decoded.Params.Progress != w.progress {
			t.Errorf("[%d] progress = %v, want %v", i, decoded.Params.Progress, w.progress)
		}
		if decoded.Params.Message != w.message {
			t.Errorf("[%d] message = %q, want %q", i, decoded.Params.Message, w.message)
		}
		if strings.Contains(string(raw), `"total"`) {
			t.Errorf("[%d] total must be omitted when unknown, got %s", i, raw)
		}
	}
}

func Test_progressReporter_silent_whenMetaAbsent(t *testing.T) {
	// Given: a session in ctx but a request without _meta
	notifications := make(chan mcp.JSONRPCNotification, 4)
	reporter := newProgressReporter(sessionContext(notifications), mcp.CallToolRequest{})

	// When: a milestone is reported
	reporter.report(progressDispatch, progressDispatchMessage("OpenRouter"))

	// Then: nothing is emitted
	if got := len(notifications); got != 0 {
		t.Fatalf("got %d notifications, want 0", got)
	}
}

func Test_progressReporter_silent_whenProgressTokenNil(t *testing.T) {
	// Given: _meta present but without a progressToken
	notifications := make(chan mcp.JSONRPCNotification, 4)
	request := mcp.CallToolRequest{Params: mcp.CallToolParams{Meta: &mcp.Meta{}}}
	reporter := newProgressReporter(sessionContext(notifications), request)

	// When: a milestone is reported
	reporter.report(progressGenerating, progressGeneratingMessage)

	// Then: nothing is emitted
	if got := len(notifications); got != 0 {
		t.Fatalf("got %d notifications, want 0", got)
	}
}

func Test_progressReporter_silent_whenNoSessionInContext(t *testing.T) {
	// Given: a token-carrying request but a bare context (no MCP session)
	reporter := newProgressReporter(context.Background(), requestWithToken("tok-1"))

	// When: a milestone is reported
	reporter.report(progressDispatch, progressDispatchMessage("APIMart"))

	// Then: the reporter stays unarmed and emits nothing
	if reporter.emit != nil {
		t.Fatal("emit must stay unset without a session in ctx")
	}
}

func Test_progressReporter_drops_whenChannelFull(t *testing.T) {
	// Given: a session whose notification channel has no reader
	blocked := make(chan mcp.JSONRPCNotification)
	reporter := newProgressReporter(sessionContext(blocked), requestWithToken("tok-1"))

	// When: a milestone is reported
	reporter.report(progressDispatch, progressDispatchMessage("APIMart"))

	// Then: the send is dropped instead of blocking the tool call
	if got := len(blocked); got != 0 {
		t.Fatalf("got %d buffered notifications, want 0", got)
	}
}
