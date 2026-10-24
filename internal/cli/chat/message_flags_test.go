package chat

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestChatPromptAliasPreservesOrder(t *testing.T) {
	tc := newChatTestCmd(t)
	if err := tc.cmd.ParseFlags([]string{"--message", "one", "--prompt", "two", "-p", "three"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if !messageFlagChanged(tc.cmd) {
		t.Fatal("messageFlagChanged() = false, want true for --prompt")
	}
	if want := []string{"one", "two", "three"}; !reflect.DeepEqual(chatMessages, want) {
		t.Fatalf("chatMessages = %v, want %v", chatMessages, want)
	}

	prevTerminal := chatStdinIsTerminal
	chatStdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { chatStdinIsTerminal = prevTerminal })

	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest() error = %v", err)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("Messages = %+v, want three user messages", req.Messages)
	}
	for i, want := range []string{"one", "two", "three"} {
		if req.Messages[i].Role != "user" || req.Messages[i].Content != want {
			t.Errorf("Messages[%d] = %+v, want user %q", i, req.Messages[i], want)
		}
	}
}

func TestChatPromptAliasOverridesJSONMessages(t *testing.T) {
	tc := newChatTestCmd(t)
	setTestFlag(t, tc.cmd.Flags(), "json", `{"model":"m","messages":[{"role":"user","content":"old"}]}`)
	setTestFlag(t, tc.cmd.Flags(), "prompt", "new")

	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest() error = %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages = %+v, want the --prompt message to replace the JSON messages", req.Messages)
	}
	if req.Messages[0].Role != "user" || req.Messages[0].Content != "new" {
		t.Errorf("Messages[0] = %+v, want user %q", req.Messages[0], "new")
	}
}

func TestChatMessageFlagsReadExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(path, []byte("from file"), 0o600); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	for _, name := range []string{"message", "prompt"} {
		t.Run(name, func(t *testing.T) {
			tc := newChatTestCmd(t)
			setTestFlag(t, tc.cmd.Flags(), name, path)

			prevTerminal := chatStdinIsTerminal
			chatStdinIsTerminal = func() bool { return true }
			t.Cleanup(func() { chatStdinIsTerminal = prevTerminal })

			req, err := buildChatRequest(tc.cmd)
			if err != nil {
				t.Fatalf("buildChatRequest() error = %v", err)
			}
			if len(req.Messages) != 1 || req.Messages[0].Content != "from file" {
				t.Errorf("Messages = %+v, want the file contents", req.Messages)
			}
		})
	}
}

func TestChatPromptDashStaysLiteral(t *testing.T) {
	tc := newChatTestCmd(t)
	setTestFlag(t, tc.cmd.Flags(), "prompt", "-")

	prevTerminal := chatStdinIsTerminal
	chatStdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { chatStdinIsTerminal = prevTerminal })

	req, err := buildChatRequest(tc.cmd)
	if err != nil {
		t.Fatalf("buildChatRequest() error = %v", err)
	}
	if len(req.Messages) != 1 || req.Messages[0].Content != "-" {
		t.Errorf("Messages = %+v, want literal \"-\"", req.Messages)
	}
}

func TestChatMessageFlagsRegistered(t *testing.T) {
	tc := newChatTestCmd(t)
	if flag := tc.cmd.Flags().Lookup("message"); flag == nil {
		t.Error("--message flag is not registered")
	}
	flag := tc.cmd.Flags().Lookup("prompt")
	if flag == nil {
		t.Fatal("--prompt flag is not registered")
	}
	if flag.Shorthand != "p" {
		t.Errorf("--prompt shorthand = %q, want %q", flag.Shorthand, "p")
	}
}
