package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveNodeAtomicWithBackup(t *testing.T) {
	path := writeFixture(t, "config.yaml", nodeFixture)
	original, _ := os.ReadFile(path)
	doc, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode() error = %v", err)
	}
	if err := SetScalar(doc, "defaults.chat.max_iterations", "10"); err != nil {
		t.Fatalf("SetScalar() error = %v", err)
	}
	if err := SaveNode(path, doc); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}

	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(backup) != string(original) {
		t.Error("backup does not match the original file")
	}
	updated, _ := os.ReadFile(path)
	if !strings.Contains(string(updated), "max_iterations: 10") {
		t.Errorf("target not updated:\n%s", updated)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp.") {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}

func TestSaveNodePreservesIndent(t *testing.T) {
	path := writeFixture(t, "config.yaml", nodeFixture)
	doc, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode() error = %v", err)
	}
	if err := SetScalar(doc, "defaults.image.model", "gpt-image-2"); err != nil {
		t.Fatalf("SetScalar() error = %v", err)
	}
	if err := SaveNode(path, doc); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "\n  image:\n    provider: agnes") {
		t.Errorf("two-space indentation not preserved:\n%s", data)
	}
}

func TestDetectIndent(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{"two spaces", "a:\n  b:\n    c: 1\n", 2},
		{"four spaces", "a:\n    b: 1\n", 4},
		{"comments skipped", "# note\n  # nested comment\na:\n  b: 1\n", 2},
		{"empty", "", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectIndent([]byte(tc.input)); got != tc.want {
				t.Errorf("detectIndent() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMarshalNodeSection(t *testing.T) {
	path := writeFixture(t, "config.yaml", nodeFixture)
	doc, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode() error = %v", err)
	}
	node, err := NodeAt(doc, "defaults.image")
	if err != nil {
		t.Fatalf("NodeAt() error = %v", err)
	}
	out, err := MarshalNode(node, 2)
	if err != nil {
		t.Fatalf("MarshalNode() error = %v", err)
	}
	if !strings.Contains(string(out), "# model comment") {
		t.Errorf("inline comment lost:\n%s", out)
	}
	if !strings.Contains(string(out), "model: agnes-image-2.5-flash") {
		t.Errorf("section value missing:\n%s", out)
	}
}
