package chat

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBindFlagsRegistersChatFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "root"}
	BindFlags(cmd)

	names := []string{
		"system", "message", "prompt", "temperature", "max-output",
		"context-size", "no-stream", "json", "interactive", "dry-run",
		"search-provider", "inject-json", "save", "session-dir", "session", "name",
	}
	for _, name := range names {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("BindFlags() did not register --%s", name)
		}
	}
}

func TestPositionalMessageAloneCountsAsMessage(t *testing.T) {
	tc := newChatTestCmd(t)
	if messageFlagChanged(tc.cmd) {
		t.Fatal("messageFlagChanged() = true before any input")
	}
	if err := addPositionalMessage([]string{"hello"}); err != nil {
		t.Fatalf("addPositionalMessage() error = %v", err)
	}
	if !messageFlagChanged(tc.cmd) {
		t.Fatal("messageFlagChanged() = false, want true for positional text alone")
	}
}

func TestAddPositionalMessageAppendedAfterFlags(t *testing.T) {
	tc := newChatTestCmd(t)
	setTestFlag(t, tc.cmd.Flags(), "prompt", "en to zh")

	if err := addPositionalMessage([]string{"hello", "world"}); err != nil {
		t.Fatalf("addPositionalMessage() error = %v", err)
	}
	if !messageFlagChanged(tc.cmd) {
		t.Fatal("messageFlagChanged() = false, want true for a positional message")
	}

	prevTerminal := chatStdinIsTerminal
	chatStdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { chatStdinIsTerminal = prevTerminal })

	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest() error = %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("Messages = %+v, want two user messages", req.Messages)
	}
	if req.Messages[0].Content != "en to zh" || req.Messages[1].Content != "hello world" {
		t.Errorf("Messages = %+v, want the flag message followed by the positional message", req.Messages)
	}
}

func TestAddPositionalMessageRejectsInteractive(t *testing.T) {
	newChatTestCmd(t)

	prev := chatInteractive
	chatInteractive = true
	t.Cleanup(func() { chatInteractive = prev })

	err := addPositionalMessage([]string{"hello"})
	if err == nil || !strings.Contains(err.Error(), "--interactive") {
		t.Fatalf("addPositionalMessage() error = %v, want the interactive conflict", err)
	}
	if len(chatMessages) != 0 {
		t.Errorf("chatMessages = %v, want no message after the rejected combination", chatMessages)
	}
}
