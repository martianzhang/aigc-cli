package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

const nodeFixture = `# top comment
api_key: sk-1234567890abcd   # inline key comment
base_url: https://api.example.com/v1

defaults:
  # image section comment
  image:
    provider: agnes
    model: agnes-image-2.5-flash  # model comment
    size: '1024x768'
  chat:
    max_iterations: 5
    temperature: 0.5
    allow_tool_override: false
`

func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestDefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}
	want := filepath.Join(home, ".config", "aigc-cli", "config.yaml")
	if path != want {
		t.Errorf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestLoadNodeMissingFile(t *testing.T) {
	_, err := LoadNode(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("LoadNode() = nil error, want read error")
	}
}

func TestNodeAt(t *testing.T) {
	path := writeFixture(t, "config.yaml", nodeFixture)
	doc, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode() error = %v", err)
	}

	t.Run("nested scalar", func(t *testing.T) {
		node, err := NodeAt(doc, "defaults.image.model")
		if err != nil {
			t.Fatalf("NodeAt() error = %v", err)
		}
		if node.Value != "agnes-image-2.5-flash" {
			t.Errorf("NodeAt() = %q", node.Value)
		}
	})

	t.Run("numeric scalar keeps type", func(t *testing.T) {
		node, err := NodeAt(doc, "defaults.chat.max_iterations")
		if err != nil {
			t.Fatalf("NodeAt() error = %v", err)
		}
		if node.Tag != "!!int" || node.Value != "5" {
			t.Errorf("NodeAt() = tag %s value %q", node.Tag, node.Value)
		}
	})

	t.Run("section returns mapping", func(t *testing.T) {
		node, err := NodeAt(doc, "defaults.image")
		if err != nil {
			t.Fatalf("NodeAt() error = %v", err)
		}
		if node.Kind != yaml.MappingNode {
			t.Errorf("NodeAt() kind = %v, want mapping", node.Kind)
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		_, err := NodeAt(doc, "defaults.image.unknown")
		if !errors.Is(err, ErrKeyNotFound) {
			t.Errorf("NodeAt() error = %v, want ErrKeyNotFound", err)
		}
	})

	t.Run("path through scalar", func(t *testing.T) {
		_, err := NodeAt(doc, "defaults.image.model.deep")
		if err == nil || errors.Is(err, ErrKeyNotFound) {
			t.Errorf("NodeAt() error = %v, want not-a-section error", err)
		}
	})

	t.Run("empty segment rejected", func(t *testing.T) {
		if _, err := NodeAt(doc, "defaults..model"); err == nil {
			t.Error("NodeAt() = nil error, want invalid key error")
		}
	})
}
