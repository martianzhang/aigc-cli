package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasExecutable(t *testing.T) {
	if !HasExecutable("sh") {
		t.Error("expected sh to be executable")
	}
	if HasExecutable("this-cmd-should-not-exist-xyz") {
		t.Error("expected unknown command to be missing")
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(txt, []byte("hello world"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"filepath": txt})
	if got := ReadFile(string(args)); !strings.Contains(got, "hello world") {
		t.Errorf("ReadFile = %q, want it to contain file content", got)
	}

	bin := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(bin, []byte("xx"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	args2, _ := json.Marshal(map[string]any{"filepath": bin})
	if got := ReadFile(string(args2)); !strings.Contains(got, "security reasons") {
		t.Errorf("ReadFile(.bin) = %q, want a security error", got)
	}
}

func TestFindFiles(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"one.txt": "1", "two.txt": "2", "three.md": "3"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	args, _ := json.Marshal(map[string]any{"pattern": "*.txt", "path": dir})
	got := FindFiles(string(args))
	if !strings.Contains(got, "one.txt") || !strings.Contains(got, "two.txt") {
		t.Errorf("FindFiles = %q, want both .txt files", got)
	}
	if strings.Contains(got, "three.md") {
		t.Errorf("FindFiles = %q, should not match three.md", got)
	}
}

func TestGrep(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("alpha\nneedle here\nbeta\n"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"pattern": "needle", "path": dir})
	if got := Grep(string(args)); !strings.Contains(got, "needle") {
		t.Errorf("Grep = %q, want a match", got)
	}
}

func TestShellCommand(t *testing.T) {
	if got := ShellCommand("echo hi"); strings.TrimSpace(got) != "hi" {
		t.Errorf("ShellCommand = %q, want hi", got)
	}
}
