package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWritePrivateCreatesOwnerOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions are not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "secret")
	if err := WritePrivate(path, []byte("data")); err != nil {
		t.Fatalf("WritePrivate() error = %v", err)
	}
	assertMode(t, path, PrivateFileMode)
}

func TestWritePrivateTightensExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions are not enforced on Windows")
	}
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := WritePrivate(path, []byte("new")); err != nil {
		t.Fatalf("WritePrivate() error = %v", err)
	}
	assertMode(t, path, PrivateFileMode)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "new" {
		t.Errorf("content = %q, want %q", data, "new")
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %04o, want %04o", path, got, want)
	}
}
