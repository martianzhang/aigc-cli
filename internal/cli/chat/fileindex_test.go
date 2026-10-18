package chat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkFiles(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("keep.txt")
	mustWrite("sub/deep.go")
	mustWrite(".hidden")
	mustWrite(".git/config")

	files := walkFiles(root)
	seen := map[string]bool{}
	for _, f := range files {
		seen[f] = true
	}
	if !seen["keep.txt"] || !seen["sub/deep.go"] {
		t.Errorf("walkFiles = %v, want keep.txt and sub/deep.go", files)
	}
	if seen[".hidden"] || seen[".git/config"] {
		t.Errorf("walkFiles = %v, should skip hidden entries", files)
	}
}

func TestSplitFileIndex(t *testing.T) {
	got := splitFileIndex("a.go\n\n  b/c.md  \n")
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b/c.md" {
		t.Errorf("splitFileIndex = %v, want [a.go b/c.md]", got)
	}
	if got := splitFileIndex(""); len(got) != 0 {
		t.Errorf("splitFileIndex(empty) = %v, want empty", got)
	}
}
