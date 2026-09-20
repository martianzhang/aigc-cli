package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfinedOutputPath_traversalRejected(t *testing.T) {
	inputDir := t.TempDir()
	input := filepath.Join(inputDir, "photo.png")

	_, err := confinedOutputPath(&Config{}, "../../evil.png", input)
	if err == nil {
		t.Fatal("expected traversal to be rejected")
	}
	if !strings.Contains(err.Error(), "escapes allowed directories") {
		t.Errorf("error = %v, want escape rejection", err)
	}
}

func TestConfinedOutputPath_absoluteOutsideRootsRejected(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	input := filepath.Join(inputDir, "photo.png")
	outside := filepath.Join(t.TempDir(), "evil.png")

	_, err := confinedOutputPath(&Config{Output: outputDir}, outside, input)
	if err == nil {
		t.Fatal("expected absolute path outside roots to be rejected")
	}
	if !strings.Contains(err.Error(), "escapes allowed directories") {
		t.Errorf("error = %v, want escape rejection", err)
	}
}

func TestConfinedOutputPath_symlinkRejected(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "photo.png")
	target := filepath.Join(t.TempDir(), "target.png")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "clean.png")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := confinedOutputPath(&Config{}, link, input)
	if err == nil {
		t.Fatal("expected symlink to be rejected")
	}
	if !strings.Contains(err.Error(), "must not be a symlink") {
		t.Errorf("error = %v, want symlink rejection", err)
	}
}

func TestConfinedOutputPath_insideOutputDirAllowed(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	input := filepath.Join(inputDir, "photo.png")
	want := filepath.Join(outputDir, "nested", "out.png")

	got, err := confinedOutputPath(&Config{Output: outputDir}, want, input)
	if err != nil {
		t.Fatalf("expected in-root output to be allowed: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConfinedOutputPath_insideInputDirAllowed(t *testing.T) {
	inputDir := t.TempDir()
	input := filepath.Join(inputDir, "photo.png")
	want := filepath.Join(inputDir, "out.png")

	got, err := confinedOutputPath(&Config{Output: t.TempDir()}, want, input)
	if err != nil {
		t.Fatalf("input dir is an allowed root: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConfinedOutputPath_relativeResolvesIntoAllowedRoot(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	input := filepath.Join(inputDir, "photo.png")

	got, err := confinedOutputPath(&Config{Output: outputDir}, "out.png", input)
	if err != nil {
		t.Fatalf("relative output_path should resolve into cfg.Output: %v", err)
	}
	if want := filepath.Join(outputDir, "out.png"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	got, err = confinedOutputPath(nil, "out.png", input)
	if err != nil {
		t.Fatalf("relative output_path should resolve next to input: %v", err)
	}
	if want := filepath.Join(inputDir, "out.png"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConfinedOutputPath_emptyKeepsDefault(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "photo.png")

	got, err := confinedOutputPath(&Config{}, "", input)
	if err != nil {
		t.Fatalf("empty output_path must keep the caller default: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty (caller default next to input)", got)
	}
}
