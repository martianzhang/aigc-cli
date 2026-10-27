package chat

import "testing"

func TestChatTitle(t *testing.T) {
	tests := []struct {
		name    string
		state   tuiState
		pending bool
		want    string
		busy    bool
	}{
		{name: "idle", state: tuiIdle, want: "chat"},
		{name: "streaming", state: tuiProcessing, want: "chat", busy: true},
		{name: "tool call", state: tuiToolCall, want: "chat · tool", busy: true},
		{name: "confirming", state: tuiConfirming, pending: true, want: "chat · confirm", busy: true},
		{name: "pending wins over state", state: tuiProcessing, pending: true, want: "chat · confirm", busy: true},
	}
	for _, tc := range tests {
		gotStage, gotBusy := chatTitle(tc.state, tc.pending)
		if gotStage != tc.want || gotBusy != tc.busy {
			t.Errorf("%s: chatTitle(%v, %v) = (%q, %v), want (%q, %v)",
				tc.name, tc.state, tc.pending, gotStage, gotBusy, tc.want, tc.busy)
		}
	}
}
