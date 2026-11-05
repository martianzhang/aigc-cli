package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func writeTestSession(t *testing.T, dir, id string, msgs ...types.ChatMessage) string {
	t.Helper()
	path := filepath.Join(dir, id+".jsonl")
	s, err := New(path, Header{ID: id})
	if err != nil {
		t.Fatalf("New(%s) error = %v", id, err)
	}
	for _, msg := range msgs {
		if err := s.AppendMessage(msg, "", 0); err != nil {
			t.Fatalf("AppendMessage(%s) error = %v", id, err)
		}
	}
	if err := s.Close("test"); err != nil {
		t.Fatalf("Close(%s) error = %v", id, err)
	}
	return path
}

func setModTime(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("Chtimes(%s) error = %v", path, err)
	}
}

func sessionIDs(list []Summary) []string {
	ids := make([]string, len(list))
	for i, s := range list {
		ids[i] = s.ID
	}
	return ids
}

func TestListSortsNewestFirstAndLimits(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	p1 := writeTestSession(t, dir, "s1", types.ChatMessage{Role: "user", Content: "one"})
	p2 := writeTestSession(t, dir, "s2", types.ChatMessage{Role: "user", Content: "two"})
	p3 := writeTestSession(t, dir, "s3", types.ChatMessage{Role: "user", Content: "three"})
	setModTime(t, p1, now.Add(-1*time.Hour))
	setModTime(t, p2, now)
	setModTime(t, p3, now.Add(-2*time.Hour))

	got, err := List(dir, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if ids := sessionIDs(got); strings.Join(ids, ",") != "s2,s1,s3" {
		t.Fatalf("List() ids = %v, want [s2 s1 s3]", ids)
	}

	limited, err := List(dir, 2)
	if err != nil {
		t.Fatalf("List(limit) error = %v", err)
	}
	if ids := sessionIDs(limited); strings.Join(ids, ",") != "s2,s1" {
		t.Fatalf("List(limit=2) ids = %v, want [s2 s1]", ids)
	}
}

func TestCount(t *testing.T) {
	dir := t.TempDir()
	writeTestSession(t, dir, "a")
	writeTestSession(t, dir, "b")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := Count(dir)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if got != 2 {
		t.Fatalf("Count() = %d, want 2", got)
	}

	missing, err := Count(filepath.Join(dir, "missing"))
	if err != nil || missing != 0 {
		t.Fatalf("Count(missing) = %d, %v; want 0, nil", missing, err)
	}
}

func TestListMissingDirectoryIsEmpty(t *testing.T) {
	got, err := List(filepath.Join(t.TempDir(), "missing"), 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() = %d entries, want 0", len(got))
	}
}

func TestSummarizeCollectsMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.jsonl")
	s, err := New(path, Header{ID: "meta", Cwd: "/tmp/work", Provider: "openai", Model: "gpt-old"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendSystem("system prompt"); err != nil {
		t.Fatal(err)
	}
	for _, msg := range []types.ChatMessage{
		{Role: "user", Content: "  hello   world  "},
		{Role: "assistant", Content: "hi"},
		{Role: "tool", Content: "tool output"},
		{Role: "user", Content: "again"},
	} {
		if err := s.AppendMessage(msg, "", 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AppendName("my session"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendModel("alt", "gpt-new", 0, "auto"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close("test"); err != nil {
		t.Fatal(err)
	}

	got, err := Summarize(path)
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if got.ID != "meta" || got.Name != "my session" || got.Cwd != "/tmp/work" {
		t.Fatalf("summary identity = %+v", got)
	}
	if got.Provider != "alt" || got.Model != "gpt-new" {
		t.Fatalf("provider/model = %q/%q, want last model record", got.Provider, got.Model)
	}
	if got.Messages != 3 {
		t.Fatalf("Messages = %d, want 3 (user+assistant+user)", got.Messages)
	}
	if got.FirstUser != "hello world" {
		t.Fatalf("FirstUser = %q", got.FirstUser)
	}
	if got.Bytes == 0 || got.UpdatedAt.IsZero() || got.CreatedAt.IsZero() {
		t.Fatalf("missing size/time metadata: %+v", got)
	}
	if got.Truncated {
		t.Fatal("Truncated = true for a small session")
	}
	if got.DisplayName() != "my session" {
		t.Fatalf("DisplayName() = %q", got.DisplayName())
	}
}

func TestSummarizeFallsBackToID(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSession(t, dir, "unnamed", types.ChatMessage{Role: "user", Content: "hi"})
	got, err := Summarize(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "" || got.DisplayName() != "unnamed" {
		t.Fatalf("Name/DisplayName = %q/%q", got.Name, got.DisplayName())
	}
}

func TestSummarizeCountsOnlyMessagesAfterCompaction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compacted.jsonl")
	s, err := New(path, Header{ID: "compacted"})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range []types.ChatMessage{
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "kept question"},
		{Role: "assistant", Content: "kept answer"},
	} {
		if err := s.AppendMessage(msg, "", 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AppendCompactionKeepLast("summary of the old turns", 2, CompactionMeta{Reason: "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close("test"); err != nil {
		t.Fatal(err)
	}

	got, err := Summarize(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Messages != 2 {
		t.Fatalf("Messages = %d, want 2 visible messages after compaction", got.Messages)
	}
	if got.FirstUser != "old question" {
		t.Fatalf("FirstUser = %q, want the original topic", got.FirstUser)
	}
}

func TestListSkipsCorruptAndNonSessionFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestSession(t, dir, "good", types.ChatMessage{Role: "user", Content: "ok"})

	if err := os.WriteFile(filepath.Join(dir, "bad.jsonl"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := List(dir, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("List() = %+v, want only the valid session", got)
	}
}

func TestSummarizeRejectsMissingHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "noheader.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"message","seq":1,"role":"user","content":"hi"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Summarize(path); err == nil {
		t.Fatal("Summarize() = nil error, want missing-header error")
	}
}

func TestResolveExisting(t *testing.T) {
	dir := t.TempDir()
	path := writeTestSession(t, dir, "exists")

	target, err := ResolveExisting("exists", dir)
	if err != nil {
		t.Fatalf("ResolveExisting(id) error = %v", err)
	}
	if target.Path != path || !target.Exists {
		t.Fatalf("ResolveExisting(id) = %+v", target)
	}

	target, err = ResolveExisting(path, dir)
	if err != nil {
		t.Fatalf("ResolveExisting(path) error = %v", err)
	}
	if target.Path != path {
		t.Fatalf("ResolveExisting(path).Path = %q", target.Path)
	}

	if _, err := ResolveExisting("missing", dir); err == nil {
		t.Fatal("ResolveExisting(missing id) = nil error")
	}
	if _, err := ResolveExisting(filepath.Join(dir, "missing.jsonl"), dir); err == nil {
		t.Fatal("ResolveExisting(missing path) = nil error")
	}
	if _, err := ResolveExisting(dir, dir); err == nil {
		t.Fatal("ResolveExisting(directory) = nil error")
	}
	if _, err := ResolveExisting("bad id!", dir); err == nil {
		t.Fatal("ResolveExisting(invalid id) = nil error")
	}
}

func TestPreviewTextTruncates(t *testing.T) {
	long := strings.Repeat("word ", 40)
	got := previewText(long)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("previewText() = %q, want ellipsis", got)
	}
	if strings.Contains(got, "  ") {
		t.Fatalf("previewText() did not collapse whitespace: %q", got)
	}
}
