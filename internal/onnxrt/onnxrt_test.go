package onnxrt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLibPath_notFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LibPath(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestLibPath_found(t *testing.T) {
	dir := t.TempDir()
	fakeLib := filepath.Join(dir, "libonnxruntime.dylib")
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

func TestLibPath_prefersGPU(t *testing.T) {
	dir := t.TempDir()
	cpuLib := filepath.Join(dir, "libonnxruntime.dylib")
	gpuLib := filepath.Join(dir, "libonnxruntime_gpu.dylib")
	os.WriteFile(cpuLib, []byte("cpu"), 0644)
	os.WriteFile(gpuLib, []byte("gpu"), 0644)

	path, err := LibPath(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != gpuLib {
		t.Fatalf("expected GPU lib, got %q", path)
	}
}

func TestVersion_const(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}
