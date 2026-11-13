package chat

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestRequireChatInput(t *testing.T) {
	prevTTY := chatStdinIsTerminal
	t.Cleanup(func() { chatStdinIsTerminal = prevTTY })

	tests := []struct {
		name        string
		messages    []types.ChatMessage
		interactive bool
		noTUI       bool
		stdinTTY    bool
		wantErr     string
	}{
		{
			name:     "message present",
			messages: []types.ChatMessage{{Role: "user", Content: "hi"}},
			stdinTTY: false,
		},
		{
			name:        "interactive requires terminal",
			interactive: true,
			stdinTTY:    false,
			wantErr:     "--interactive requires a terminal",
		},
		{
			name:     "empty stdin rejected",
			stdinTTY: false,
			wantErr:  "no input",
		},
		{
			name:     "no-tui without message rejected",
			noTUI:    true,
			stdinTTY: true,
			wantErr:  "no input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatStdinIsTerminal = func() bool { return tt.stdinTTY }
			err := requireChatInput(tt.messages, tt.interactive, tt.noTUI)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("requireChatInput() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("requireChatInput() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// TestRunChatEmptyStdinReturnsPromptly guards against the observed hang:
// `aigc-cli chat </dev/null` used to send an empty request. With no message and
// a non-TTY stdin it must fail immediately.
func TestRunChatEmptyStdinReturnsPromptly(t *testing.T) {
	prevTTY, prevStdin, prevMessages, prevJSON, prevDryRun, prevNoTUI :=
		chatStdinIsTerminal, chatStdin, chatMessages, chatJSONFlag, chatDryRun, chatNoTUI
	t.Cleanup(func() {
		chatStdinIsTerminal, chatStdin, chatMessages, chatJSONFlag, chatDryRun, chatNoTUI =
			prevTTY, prevStdin, prevMessages, prevJSON, prevDryRun, prevNoTUI
	})

	chatStdinIsTerminal = func() bool { return false }
	chatStdin = strings.NewReader("")
	chatMessages = nil
	chatJSONFlag = ""
	chatDryRun = false
	chatNoTUI = true

	cmd := &cobra.Command{Use: "chat"}
	done := make(chan error, 1)
	go func() { done <- runChat(cmd, nil) }()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "no input") {
			t.Fatalf("runChat() error = %v, want no input", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runChat() did not return; it may be waiting for input")
	}
}
