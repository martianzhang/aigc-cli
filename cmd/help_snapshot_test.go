package cmd

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateHelpSnapshot = flag.Bool("update", false, "update golden help snapshots")

// TestRootHelpSnapshot pins the root help text. Flag/description drift is a
// user-visible change and should be reviewed in the diff.
func TestRootHelpSnapshot(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	t.Cleanup(func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
	})
	rootCmd.HelpFunc()(rootCmd, nil)

	path := filepath.Join("testdata", "root_help.golden")
	if *updateHelpSnapshot {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run: go test ./cmd -run TestRootHelpSnapshot -update): %v", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("root help changed; review and update with: go test ./cmd -run TestRootHelpSnapshot -update\n--- got ---\n%s", buf.String())
	}
}
