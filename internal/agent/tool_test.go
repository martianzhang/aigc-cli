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

	// Default policy allows any file (e.g. .bin, .mod, extensionless).
	bin := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(bin, []byte("xx"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	args2, _ := json.Marshal(map[string]any{"filepath": bin})
	if got := ReadFile(string(args2)); !strings.Contains(got, "xx") {
		t.Errorf("ReadFile(.bin) = %q, want content (allow-all default)", got)
	}
}

func TestReadFileWithPolicy(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(secret, []byte("PRIVATE"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"filepath": secret})

	// Deny by base name.
	got := ReadFileWithPolicy(string(args), []string{`^id_rsa$`})
	if !strings.Contains(got, "blocked by defaults.chat.read_deny") {
		t.Errorf("ReadFileWithPolicy(deny) = %q, want blocked", got)
	}

	// Deny by extension; unrelated files still readable.
	tmp := filepath.Join(dir, "note.md")
	_ = os.WriteFile(tmp, []byte("ok"), 0644)
	argsMD, _ := json.Marshal(map[string]any{"filepath": tmp})
	gotMD := ReadFileWithPolicy(string(argsMD), []string{`\.pem$`, `^id_rsa$`})
	if !strings.Contains(gotMD, "ok") {
		t.Errorf("ReadFileWithPolicy(allow) = %q, want content", gotMD)
	}

	// Non-matching pattern allows everything.
	if got := ReadFileWithPolicy(string(args), []string{`\.pem$`}); !strings.Contains(got, "PRIVATE") {
		t.Errorf("ReadFileWithPolicy(non-match) = %q, want content", got)
	}
}

func TestReadFile_binary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "image.png")
	data := []byte{0x89, 'P', 'N', 'G', 0x00, 0x1a, 0x00, 0x00, 0xff, 0xfe}
	if err := os.WriteFile(bin, data, 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"filepath": bin})
	got := ReadFile(string(args))
	if !strings.Contains(got, "binary file") {
		t.Errorf("ReadFile(binary) = %q, want binary notice", got)
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
