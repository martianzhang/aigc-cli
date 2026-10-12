package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "out.txt")

	got := WriteFile(`{"filepath":"` + path + `","content":"hello world"}`)
	if !strings.Contains(got, "Wrote") {
		t.Fatalf("WriteFile = %q", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("content = %q", string(data))
	}
}

func TestWriteFile_requiresFilepath(t *testing.T) {
	if got := WriteFile(`{"content":"x"}`); !strings.Contains(got, "filepath is required") {
		t.Errorf("WriteFile = %q", got)
	}
}

func TestEditFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("foo bar baz"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := EditFile(`{"filepath":"` + path + `","old_string":"bar","new_string":"QUX"}`)
	if !strings.Contains(got, "Edited") {
		t.Fatalf("EditFile = %q", got)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "foo QUX baz" {
		t.Errorf("content = %q", string(data))
	}

	if got := EditFile(`{"filepath":"` + path + `","old_string":"nope","new_string":"x"}`); !strings.Contains(got, "not found") {
		t.Errorf("missing old_string = %q", got)
	}
}

func TestEditFile_notUnique(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("aa aa"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := EditFile(`{"filepath":"` + path + `","old_string":"aa","new_string":"b"}`)
	if !strings.Contains(got, "not unique") {
		t.Errorf("EditFile = %q", got)
	}
}

func TestListDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := ListDir(`{"path":"` + dir + `"}`)
	if !strings.Contains(got, "a.txt") || !strings.Contains(got, "sub/") {
		t.Errorf("ListDir = %q", got)
	}
}

func TestListDir_missing(t *testing.T) {
	got := ListDir(`{"path":"/nonexistent/definitely/not/here"}`)
	if !strings.Contains(got, "Error") {
		t.Errorf("ListDir = %q", got)
	}
}

func TestBash(t *testing.T) {
	got := Bash(`{"command":"echo hi"}`)
	if !strings.Contains(got, "hi") {
		t.Errorf("Bash = %q", got)
	}
	if got := Bash(`{"command":"  "}`); !strings.Contains(got, "command is required") {
		t.Errorf("Bash empty = %q", got)
	}
}
