package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentsContextFrom(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("ROOT-CONTEXT"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "AGENTS.md"), []byte("MID-CONTEXT"), 0o644); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(root, "global-AGENTS.md")
	if err := os.WriteFile(global, []byte("GLOBAL-CONTEXT"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := agentsContextFrom(sub, global)
	for _, want := range []string{"GLOBAL-CONTEXT", "ROOT-CONTEXT", "MID-CONTEXT"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Order: global, then outermost project, then innermost.
	gIdx := strings.Index(got, "GLOBAL-CONTEXT")
	rIdx := strings.Index(got, "ROOT-CONTEXT")
	mIdx := strings.Index(got, "MID-CONTEXT")
	if gIdx >= rIdx || rIdx >= mIdx {
		t.Errorf("unexpected order (global=%d root=%d mid=%d):\n%s", gIdx, rIdx, mIdx, got)
	}
}

func TestAgentsContextFrom_none(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := agentsContextFrom(dir, ""); got != "" {
		t.Errorf("expected empty context, got %q", got)
	}
}
