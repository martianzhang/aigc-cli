package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/martianzhang/aigc-cli/internal/fsutil"
)

// SaveNode writes doc back to path. The current file is copied to
// path+".bak" first, the new content goes to path+".tmp.<pid>", and that temp
// file is renamed over path so an interrupted write cannot corrupt the config.
// The config can hold API keys, so it and its backup are always written with
// owner-only permissions (0600), even if the existing file was more permissive.
func SaveNode(path string, doc *yaml.Node) error {
	original, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if err := fsutil.WritePrivate(path+".bak", original); err != nil {
		return fmt.Errorf("write config backup: %w", err)
	}

	data, err := MarshalNode(doc, detectIndent(original))
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp.%d", path, os.Getpid())
	if err := fsutil.WritePrivate(tmp, data); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// MarshalNode encodes a YAML node tree, preserving comments and key order.
// Indents outside yaml's supported range fall back to two spaces.
func MarshalNode(node *yaml.Node, indent int) ([]byte, error) {
	if indent < 2 || indent > 9 {
		indent = 2
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(indent)
	if err := enc.Encode(node); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return buf.Bytes(), nil
}

// detectIndent returns the first nested indentation width found in data,
// falling back to two spaces (the project's config style).
func detectIndent(data []byte) int {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" || trimmed == line || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if width := len(line) - len(trimmed); width >= 1 && width <= 9 {
			return width
		}
	}
	return 2
}
