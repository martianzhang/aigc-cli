package chat

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestWriteSessionList(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := createSessionFile(t, dir, "old-id", "Old topic", "", "model-a",
		types.ChatMessage{Role: "user", Content: "old question"})
	fresh := createSessionFile(t, dir, "fresh-id", "", "", "model-b",
		types.ChatMessage{Role: "user", Content: "fresh question"})
	if err := os.Chtimes(old, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fresh, now, now); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeSessionList(&buf, dir, 0); err != nil {
		t.Fatalf("writeSessionList() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Sessions in " + dir,
		"Old topic",
		"old-id",
		"fresh-id",
		"model-a",
		"model-b",
		"old question",
		"fresh question",
		"Resume: aigc-cli chat --session <id>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "fresh-id") > strings.Index(out, "old-id") {
		t.Errorf("sessions are not newest-first:\n%s", out)
	}
}

func TestWriteSessionListEmpty(t *testing.T) {
	var buf bytes.Buffer
	dir := t.TempDir()
	if err := writeSessionList(&buf, dir, 0); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "No saved sessions in "+dir) {
		t.Fatalf("empty output = %q", got)
	}
}

func TestWriteSessionListLimit(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	first := createSessionFile(t, dir, "first", "", "", "", types.ChatMessage{Role: "user", Content: "1"})
	second := createSessionFile(t, dir, "second", "", "", "", types.ChatMessage{Role: "user", Content: "2"})
	if err := os.Chtimes(first, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(second, now, now); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeSessionList(&buf, dir, 1); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "second") || strings.Contains(out, "first") {
		t.Fatalf("limit=1 output = %q", out)
	}
	if !strings.Contains(out, "Showing the 1 most recent of 2 sessions") {
		t.Fatalf("limit footer missing from %q", out)
	}
}

func TestRunListSessionsRespectsSessionDir(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	dir := t.TempDir()
	createSessionFile(t, dir, "listed", "Listed session", "", "m", types.ChatMessage{Role: "user", Content: "hello"})

	oldDir, oldList, oldLimit := chatSessionDir, chatListSessions, chatSessionLimit
	chatSessionDir, chatListSessions, chatSessionLimit = dir, true, sessionListLimit
	t.Cleanup(func() { chatSessionDir, chatListSessions, chatSessionLimit = oldDir, oldList, oldLimit })

	cmd := &cobra.Command{}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runListSessions(cmd); err != nil {
		t.Fatalf("runListSessions() error = %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "Listed session") || !strings.Contains(got, "listed") {
		t.Fatalf("output = %q", got)
	}
}

func TestRunChatListSessionsSkipsProvider(t *testing.T) {
	withChatConfig(t, nil, 0, 0, 0)
	dir := t.TempDir()
	createSessionFile(t, dir, "wired", "", "", "m", types.ChatMessage{Role: "user", Content: "hello"})

	oldDir, oldList, oldLimit := chatSessionDir, chatListSessions, chatSessionLimit
	chatSessionDir, chatListSessions, chatSessionLimit = "", false, sessionListLimit
	t.Cleanup(func() { chatSessionDir, chatListSessions, chatSessionLimit = oldDir, oldList, oldLimit })

	cmd := &cobra.Command{RunE: runChat}
	cmd.Flags().BoolVar(&chatListSessions, "list-sessions", false, "")
	cmd.Flags().IntVar(&chatSessionLimit, "session-limit", sessionListLimit, "")
	cmd.Flags().StringVar(&chatSessionDir, "session-dir", "", "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.Flags().Parse([]string{"--list-sessions", "--session-dir", dir}); err != nil {
		t.Fatal(err)
	}
	if err := runChat(cmd, nil); err != nil {
		t.Fatalf("runChat(--list-sessions) error = %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "wired") {
		t.Fatalf("output = %q", got)
	}
}

func TestValidateListSessionsNegativeLimit(t *testing.T) {
	oldLimit, oldMessages := chatSessionLimit, chatMessages
	chatSessionLimit, chatMessages = -1, nil
	t.Cleanup(func() { chatSessionLimit, chatMessages = oldLimit, oldMessages })

	if err := validateListSessions(&cobra.Command{}); err == nil || !strings.Contains(err.Error(), "--session-limit") {
		t.Fatalf("validateListSessions() = %v, want a --session-limit error", err)
	}
}

func TestValidateListSessionsConflicts(t *testing.T) {
	for _, name := range []string{"save", "session", "name", "json", "dry-run", "interactive"} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool(name, false, "")
			if err := cmd.Flags().Set(name, "true"); err != nil {
				t.Fatal(err)
			}
			if err := validateListSessions(cmd); err == nil || !strings.Contains(err.Error(), "--"+name) {
				t.Fatalf("validateListSessions() = %v, want conflict with --%s", err, name)
			}
		})
	}

	oldMessages := chatMessages
	chatMessages = []string{"hello"}
	t.Cleanup(func() { chatMessages = oldMessages })
	if err := validateListSessions(&cobra.Command{}); err == nil {
		t.Fatal("validateListSessions() accepted a message")
	}
}
