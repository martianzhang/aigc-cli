package onnxrt

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLibPath_notFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LibPath(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func platformLibName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libonnxruntime.dylib"
	case "linux":
		return "libonnxruntime.so"
	default:
		return "onnxruntime.dll"
	}
}

func TestLibPath_found(t *testing.T) {
	dir := t.TempDir()
	libName := platformLibName()
	fakeLib := filepath.Join(dir, libName)
	if err := os.WriteFile(fakeLib, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	path, err := LibPath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != fakeLib {
		t.Fatalf("got %q, want %q", path, fakeLib)
	}
}

func TestLibPath_prefersCPUOnly(t *testing.T) {
	dir := t.TempDir()
	// LibPath only looks for the single main library per platform.
	// GPU providers are loaded dynamically alongside it.
	libName := platformLibName()
	lib := filepath.Join(dir, libName)
	os.WriteFile(lib, []byte("data"), 0644)

	path, err := LibPath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != lib {
		t.Fatalf("got %q, want %q", path, lib)
	}
}

func TestVersion_const(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}
