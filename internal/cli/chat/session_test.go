package chat

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func resetSessionGlobals(t *testing.T) {
	t.Helper()
	oldSave, oldDir, oldSession, oldSystem := chatSave, chatSessionDir, chatSession, chatSystem
	oldName, oldJSON, oldDryRun := chatName, chatJSONFlag, chatDryRun
	t.Cleanup(func() {
		chatSave, chatSessionDir, chatSession, chatSystem = oldSave, oldDir, oldSession, oldSystem
		chatName, chatJSONFlag, chatDryRun = oldName, oldJSON, oldDryRun
	})
	chatSave, chatSessionDir, chatSession, chatSystem = false, "", "", ""
	chatName, chatJSONFlag, chatDryRun = "", "", false
}

func TestValidateSessionFlags(t *testing.T) {
	tests := []struct {
		name        string
		save        bool
		session     string
		system      string
		sessionName string
		json        string
		dryRun      bool
		wantErr     bool
	}{
		{name: "save only", save: true},
		{name: "session only", session: "id"},
		{name: "name without session", sessionName: "n", wantErr: true},
		{name: "name with save", save: true, sessionName: "n"},
		{name: "name with session", session: "id", sessionName: "n"},
		{name: "save and session", save: true, session: "id", wantErr: true},
		{name: "json and save", save: true, json: "{}", wantErr: true},
		{name: "json and session", session: "id", json: "{}", wantErr: true},
		{name: "dry run and save", save: true, dryRun: true, wantErr: true},
		{name: "dry run and session", session: "id", dryRun: true},
		{name: "system and new session", session: "id", system: "sys"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetSessionGlobals(t)
			chatSave, chatSession, chatSystem = tc.save, tc.session, tc.system
			chatName, chatJSONFlag, chatDryRun = tc.sessionName, tc.json, tc.dryRun
			err := validateSessionFlags()
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateSessionFlags() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestBuildHistoryFromSessionPersistsOnce(t *testing.T) {
	resetSessionGlobals(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	sess, err := session.New(path, session.Header{ID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close("exit")

	if err := sess.AppendSystem("sys"); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendMessage(types.ChatMessage{Role: "user", Content: "old"}, "", 0); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendMessage(types.ChatMessage{Role: "assistant", Content: "old answer"}, "", 0); err != nil {
		t.Fatal(err)
	}

	history, err := buildHistoryFromSession(sess, []types.ChatMessage{
		{Role: "user", Content: "new"},
	}, nil, true)
	if err != nil {
		t.Fatalf("buildHistoryFromSession() error = %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("history len = %d, want 4 (system + 2 old + 1 new): %+v", len(history), history)
	}
	if history[3].Content != "new" {
		t.Errorf("last message = %q, want new", history[3].Content)
	}

	loaded, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(loaded.History()); got != 4 {
		t.Errorf("persisted history len = %d, want 4", got)
	}
}

func TestBuildHistoryFromSessionReadOnlyAddsMessageOnce(t *testing.T) {
	resetSessionGlobals(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	sess, err := session.New(path, session.Header{ID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendSystem("sys"); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendMessage(types.ChatMessage{Role: "user", Content: "old"}, "", 0); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close("exit"); err != nil {
		t.Fatal(err)
	}

	readOnly, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	history, err := buildHistoryFromSession(readOnly, []types.ChatMessage{
		{Role: "user", Content: "new"},
	}, nil, false)
	if err != nil {
		t.Fatalf("buildHistoryFromSession() error = %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("history len = %d, want 3: %+v", len(history), history)
	}
	if history[1].Content != "old" || history[2].Content != "new" {
		t.Errorf("history = %+v, want old then new", history)
	}

	file, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(file), `"content":"new"`) != 0 {
		t.Error("read-only history build wrote the new message to disk")
	}
}

func TestBuildHistoryFromSessionRejectsSystemWhenStored(t *testing.T) {
	resetSessionGlobals(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	s, err := session.New(path, session.Header{ID: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendSystem("stored"); err != nil {
		t.Fatal(err)
	}
	if _, err := buildHistoryFromSession(s, []types.ChatMessage{
		{Role: "system", Content: "cli"},
		{Role: "user", Content: "hi"},
	}, nil, true); err == nil {
		t.Fatal("buildHistoryFromSession() error = nil, want system conflict")
	}
}

func TestSplitLeadingSystem(t *testing.T) {
	system, messages := splitLeadingSystem([]types.ChatMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
	})
	if system != "sys" || len(messages) != 1 || messages[0].Content != "hi" {
		t.Errorf("splitLeadingSystem() = %q, %+v", system, messages)
	}
	system, messages = splitLeadingSystem([]types.ChatMessage{{Role: "user", Content: "hi"}})
	if system != "" || len(messages) != 1 {
		t.Errorf("splitLeadingSystem(no system) = %q, %+v", system, messages)
	}
}

func TestSessionToolOutputMax(t *testing.T) {
	if got := sessionToolOutputMax(nil); got != session.DefaultToolOutputMaxBytes {
		t.Errorf("default = %d, want %d", got, session.DefaultToolOutputMaxBytes)
	}
	if got := sessionToolOutputMax(&types.ChatDefaults{SessionToolOutputMaxBytes: 123}); got != 123 {
		t.Errorf("configured = %d, want 123", got)
	}
}

func TestSessionTitle(t *testing.T) {
	if got := (&chatModel{}).sessionTitle(); got != "Chat with AI" {
		t.Errorf("no session title = %q", got)
	}

	resetSessionGlobals(t)
	path := filepath.Join(t.TempDir(), "title.jsonl")
	sess, err := session.New(path, session.Header{ID: "title-id"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close("exit")
	m := &chatModel{sess: sess}
	if got, want := m.sessionTitle(), "Session title-id"; got != want {
		t.Errorf("unnamed title = %q, want %q", got, want)
	}
	if err := sess.AppendName("my project"); err != nil {
		t.Fatal(err)
	}
	if got, want := m.sessionTitle(), "my project"; got != want {
		t.Errorf("named title = %q, want %q", got, want)
	}
}

func TestSetSessionName(t *testing.T) {
	resetSessionGlobals(t)
	path := filepath.Join(t.TempDir(), "rename.jsonl")
	sess, err := session.New(path, session.Header{ID: "rename"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close("exit")
	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.sess = sess

	m.setSessionName("  my   project ")
	if got, want := sess.Name(), "my project"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].content, "Session name set") {
		t.Errorf("confirmation message missing: %+v", m.messages)
	}

	noSession := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	noSession.setSessionName("x")
	if len(noSession.messages) == 0 || !strings.Contains(noSession.messages[len(noSession.messages)-1].content, "No active session") {
		t.Errorf("no-session message missing: %+v", noSession.messages)
	}
}

func TestAutoSessionName(t *testing.T) {
	tests := []struct {
		name    string
		history []types.ChatMessage
		want    string
	}{
		{
			name:    "first user message",
			history: []types.ChatMessage{{Role: "user", Content: "Review the auth flow"}},
			want:    "Review the auth flow",
		},
		{
			name: "skips short greeting",
			history: []types.ChatMessage{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello"},
				{Role: "user", Content: "Investigate the crash"},
			},
			want: "Investigate the crash",
		},
		{
			name:    "strips file block",
			history: []types.ChatMessage{{Role: "user", Content: `<file name="a.go">package main</file> review this`}},
			want:    "review this",
		},
		{
			name:    "strips image marker",
			history: []types.ChatMessage{{Role: "user", Content: "[attached image: cat.png] describe this"}},
			want:    "describe this",
		},
		{
			name:    "no user message",
			history: []types.ChatMessage{{Role: "assistant", Content: "hello"}},
			want:    "",
		},
		{
			name:    "long message is truncated",
			history: []types.ChatMessage{{Role: "user", Content: strings.Repeat("a", 100)}},
			want:    strings.Repeat("a", maxSessionNameRunes) + "…",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := autoSessionName(tc.history); got != tc.want {
				t.Errorf("autoSessionName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShowSessionNameGeneratesForUnnamedSession(t *testing.T) {
	resetSessionGlobals(t)
	path := filepath.Join(t.TempDir(), "auto-name.jsonl")
	sess, err := session.New(path, session.Header{ID: "auto-name"})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close("exit")

	m := newChatModel(nil, nil, 10, "m", "", nil, false, 0, 0, 0)
	m.sess = sess
	m.history = []types.ChatMessage{{Role: "user", Content: "Review the auth flow"}}

	m.showSessionName()
	if got, want := sess.Name(), "Review the auth flow"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].content, "Session name set") {
		t.Errorf("confirmation message missing: %+v", m.messages)
	}

	// A second call shows the current identity instead of regenerating.
	m.showSessionName()
	last := m.messages[len(m.messages)-1].content
	if !strings.Contains(last, "Session: auto-name") || !strings.Contains(last, "Review the auth flow") {
		t.Errorf("current-session message = %q", last)
	}
}

func TestResumedChatMessagesSkipsSystemAndCaps(t *testing.T) {
	history := []types.ChatMessage{{Role: "system", Content: "prompt"}}
	for i := 0; i < maxResumeDisplayMessages+5; i++ {
		history = append(history, types.ChatMessage{Role: "user", Content: "msg"})
	}
	got := resumedChatMessages(history)
	if len(got) != maxResumeDisplayMessages+1 {
		t.Fatalf("len = %d, want %d", len(got), maxResumeDisplayMessages+1)
	}
	if !strings.Contains(got[0].content, "Showing the last") {
		t.Errorf("first message = %q, want truncation notice", got[0].content)
	}
	for _, msg := range got {
		if msg.content == "prompt" {
			t.Error("system prompt was rendered into the transcript")
		}
	}
}

func TestResumeCommand(t *testing.T) {
	newSession := func(t *testing.T, dir, id string) *session.Session {
		t.Helper()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		sess, err := session.New(filepath.Join(dir, id+".jsonl"), session.Header{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if err := sess.Close("exit"); err != nil {
			t.Fatal(err)
		}
		return sess
	}

	t.Run("default directory uses id", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("HOME override is not portable on Windows")
		}
		home := t.TempDir()
		t.Setenv("HOME", home)
		resetSessionGlobals(t)
		dir := filepath.Join(home, ".config", "aigc-cli", "sessions")
		sess := newSession(t, dir, "abc")
		if got, want := resumeCommand(sess, nil), "aigc-cli chat --session abc"; got != want {
			t.Errorf("resumeCommand() = %q, want %q", got, want)
		}
	})

	t.Run("custom directory is repeated", func(t *testing.T) {
		resetSessionGlobals(t)
		dir := filepath.Join(t.TempDir(), "custom sessions")
		chatSessionDir = dir
		sess := newSession(t, dir, "def")
		got := resumeCommand(sess, nil)
		want := "aigc-cli chat --session def --session-dir '" + dir + "'"
		if got != want {
			t.Errorf("resumeCommand() = %q, want %q", got, want)
		}
	})

	t.Run("explicit path is used as-is", func(t *testing.T) {
		resetSessionGlobals(t)
		path := filepath.Join(t.TempDir(), "review.jsonl")
		sess := newSession(t, filepath.Dir(path), "review")
		got := resumeCommand(sess, nil)
		want := "aigc-cli chat --session '" + path + "'"
		if got != want {
			t.Errorf("resumeCommand() = %q, want %q", got, want)
		}
	})
}
