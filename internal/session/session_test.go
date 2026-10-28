package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func newTestSession(t *testing.T, id string) (*Session, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, id+".jsonl")
	s, err := New(path, Header{ID: id, Model: "test-model"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return s, path
}

func TestNewAppendLoadHistory(t *testing.T) {
	s, path := newTestSession(t, "s1")
	if err := s.AppendSystem("system prompt"); err != nil {
		t.Fatalf("AppendSystem() error = %v", err)
	}
	if err := s.AppendMessage(types.ChatMessage{
		Role:    "user",
		Content: "hello",
		Images: []types.ChatImage{{
			Name:     "cat.png",
			MIMEType: "image/png",
			Data:     []byte("not-stored-bytes"),
		}},
	}, "", 0); err != nil {
		t.Fatalf("AppendMessage(user) error = %v", err)
	}
	if err := s.AppendMessage(types.ChatMessage{
		Role:    "assistant",
		Content: "hi",
		ToolCalls: []types.ToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: types.ToolCallFunction{
				Name:      "read",
				Arguments: `{"path":"a.go"}`,
			},
		}},
	}, "", 0); err != nil {
		t.Fatalf("AppendMessage(assistant) error = %v", err)
	}
	if err := s.Close("exit"); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// The current process keeps image bytes for the live request; a reloaded
	// session only has metadata.
	if got := len(s.History()[1].Images); got != 1 {
		t.Errorf("in-memory images = %d, want 1", got)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.ID() != "s1" {
		t.Errorf("ID() = %q, want s1", loaded.ID())
	}
	if loaded.System() != "system prompt" {
		t.Errorf("System() = %q", loaded.System())
	}
	hist := loaded.History()
	if len(hist) != 3 {
		t.Fatalf("History() len = %d, want 3", len(hist))
	}
	if hist[0].Role != "system" || hist[0].Content != "system prompt" {
		t.Errorf("history[0] = %+v", hist[0])
	}
	if hist[1].Role != "user" || hist[1].Content != "hello" {
		t.Errorf("history[1] = %+v", hist[1])
	}
	if hist[2].Role != "assistant" || len(hist[2].ToolCalls) != 1 {
		t.Errorf("history[2] = %+v", hist[2])
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "not-stored-bytes") {
		t.Error("image bytes were written to the session file")
	}
	if !strings.Contains(string(raw), `"name":"cat.png"`) {
		t.Error("image metadata is missing from the session file")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("mode = %o, want 600", perm)
		}
	}
}

func TestCompactionProjectsSummaryAndTail(t *testing.T) {
	s, path := newTestSession(t, "compact")
	mustAppendSystem(t, s, "sys")
	mustAppendMessage(t, s, "user", "old question")
	mustAppendMessage(t, s, "assistant", "old answer")
	if err := s.AppendCompaction("the summary", 100, 10, "m"); err != nil {
		t.Fatalf("AppendCompaction() error = %v", err)
	}
	mustAppendMessage(t, s, "user", "new question")
	mustAppendMessage(t, s, "assistant", "new answer")
	if err := s.Close("exit"); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	hist := loaded.History()
	if len(hist) != 4 {
		t.Fatalf("History() len = %d, want 4: %+v", len(hist), hist)
	}
	if hist[0].Content != "sys" {
		t.Errorf("history[0] = %q", hist[0].Content)
	}
	if !strings.Contains(hist[1].Content, "the summary") {
		t.Errorf("history[1] = %q, want summary", hist[1].Content)
	}
	if hist[2].Content != "new question" || hist[3].Content != "new answer" {
		t.Errorf("tail = %+v", hist[2:])
	}
	if strings.Contains(strings.Join(historyContents(hist), "|"), "old question") {
		t.Error("pre-compaction message leaked into resumed history")
	}
}

func TestLatestCompactionWins(t *testing.T) {
	s, path := newTestSession(t, "compact2")
	mustAppendSystem(t, s, "sys")
	mustAppendMessage(t, s, "user", "one")
	if err := s.AppendCompaction("first", 10, 5, "m"); err != nil {
		t.Fatal(err)
	}
	mustAppendMessage(t, s, "assistant", "two")
	if err := s.AppendCompaction("second", 20, 5, "m"); err != nil {
		t.Fatal(err)
	}
	mustAppendMessage(t, s, "user", "three")
	if err := s.Close("exit"); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	hist := loaded.History()
	if len(hist) != 3 {
		t.Fatalf("History() len = %d, want 3: %+v", len(hist), hist)
	}
	if !strings.Contains(hist[1].Content, "second") || strings.Contains(hist[1].Content, "first") {
		t.Errorf("summary = %q, want latest only", hist[1].Content)
	}
	if hist[2].Content != "three" {
		t.Errorf("tail = %+v", hist[2])
	}
}

func TestClearBarrier(t *testing.T) {
	s, path := newTestSession(t, "clear")
	mustAppendSystem(t, s, "sys")
	mustAppendMessage(t, s, "user", "before")
	if err := s.AppendClear(); err != nil {
		t.Fatalf("AppendClear() error = %v", err)
	}
	mustAppendMessage(t, s, "user", "after")
	if err := s.Close("exit"); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	hist := loaded.History()
	if len(hist) != 2 || hist[0].Content != "sys" || hist[1].Content != "after" {
		t.Fatalf("History() = %+v, want [sys, after]", hist)
	}
}

func TestToolOutputTruncation(t *testing.T) {
	s, path := newTestSession(t, "truncate")
	mustAppendSystem(t, s, "sys")
	long := strings.Repeat("a", 100)
	if err := s.AppendMessage(types.ChatMessage{Role: "tool", Content: long}, "bash", 20); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendMessage(types.ChatMessage{Role: "assistant", Content: long}, "", 20); err != nil {
		t.Fatal(err)
	}
	if err := s.Close("exit"); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	hist := loaded.History()
	if len(hist) != 3 {
		t.Fatalf("History() len = %d", len(hist))
	}
	if !strings.Contains(hist[1].Content, "[truncated:") {
		t.Errorf("tool content not truncated: %q", hist[1].Content)
	}
	if hist[2].Content != long {
		t.Errorf("non-tool content was truncated")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("bad line: %v", err)
		}
		records = append(records, rec)
	}
	var found bool
	for _, rec := range records {
		if rec["type"] == "message" && rec["role"] == "tool" {
			found = true
			if rec["truncated"] != true {
				t.Errorf("truncated = %v, want true", rec["truncated"])
			}
			if rec["original_bytes"] != float64(100) {
				t.Errorf("original_bytes = %v, want 100", rec["original_bytes"])
			}
			if rec["name"] != "bash" {
				t.Errorf("name = %v, want bash", rec["name"])
			}
		}
	}
	if !found {
		t.Fatal("tool record not found")
	}
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	id := "20261028T101500Z-ab12cd"
	path := filepath.Join(dir, id+".jsonl")
	s, err := New(path, Header{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close("exit"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		value  string
		want   string
		exists bool
		err    bool
	}{
		{"existing id", id, path, true, false},
		{"existing path", path, path, true, false},
		{"missing id", "does-not-exist", "", false, true},
		{"new explicit path", filepath.Join(dir, "new.jsonl"), filepath.Join(dir, "new.jsonl"), false, false},
		{"new path in missing dir", filepath.Join(dir, "nested", "new.jsonl"), filepath.Join(dir, "nested", "new.jsonl"), false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.value, dir)
			if tc.err {
				if err == nil {
					t.Fatalf("Resolve() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got.Path != tc.want {
				t.Errorf("Path = %q, want %q", got.Path, tc.want)
			}
			if got.Exists != tc.exists {
				t.Errorf("Exists = %v, want %v", got.Exists, tc.exists)
			}
		})
	}
}

func TestLoadIgnoresMalformedTrailingLine(t *testing.T) {
	s, path := newTestSession(t, "tail")
	mustAppendSystem(t, s, "sys")
	mustAppendMessage(t, s, "user", "hi")
	if err := s.Close("exit"); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"message"`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want trailing-line tolerance", err)
	}
	if got := len(loaded.History()); got != 2 {
		t.Errorf("History() len = %d, want 2", got)
	}
}

func TestLoadRejectsMalformedMiddleLine(t *testing.T) {
	s, path := newTestSession(t, "middle")
	mustAppendSystem(t, s, "sys")
	if err := s.Close("exit"); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{bad}\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"message","seq":99,"role":"user","content":"later"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil, want malformed-middle error")
	}
}

func TestNewID(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 15, 0, 0, time.UTC)
	id := NewID(now)
	if !strings.HasPrefix(id, "20261028T101500Z-") {
		t.Errorf("NewID() = %q, want timestamp prefix", id)
	}
	if !idPattern.MatchString(id) {
		t.Errorf("NewID() = %q, not a valid id", id)
	}
	first, second := NewID(now), NewID(now)
	if first == second {
		t.Error("NewID() returned the same value twice")
	}
}

func mustAppendSystem(t *testing.T, s *Session, content string) {
	t.Helper()
	if err := s.AppendSystem(content); err != nil {
		t.Fatalf("AppendSystem() error = %v", err)
	}
}

func mustAppendMessage(t *testing.T, s *Session, role, content string) {
	t.Helper()
	if err := s.AppendMessage(types.ChatMessage{Role: role, Content: content}, "", 0); err != nil {
		t.Fatalf("AppendMessage(%s) error = %v", role, err)
	}
}

func historyContents(hist []types.ChatMessage) []string {
	out := make([]string, 0, len(hist))
	for _, m := range hist {
		out = append(out, m.Content)
	}
	return out
}

func TestSessionNameLatestWins(t *testing.T) {
	s, path := newTestSession(t, "named")
	if err := s.AppendName("first"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendName("  second\n name "); err != nil {
		t.Fatal(err)
	}
	if got, want := s.Name(), "second name"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
	if err := s.Close("exit"); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := loaded.Name(), "second name"; got != want {
		t.Errorf("loaded Name() = %q, want %q", got, want)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"type":"session_info"`) {
		t.Error("session_info record missing")
	}
}

func TestSessionNameEmptyIsIgnored(t *testing.T) {
	s, _ := newTestSession(t, "empty-name")
	if err := s.AppendName("   "); err != nil {
		t.Fatal(err)
	}
	if got := s.Name(); got != "" {
		t.Errorf("Name() = %q, want empty", got)
	}
}
