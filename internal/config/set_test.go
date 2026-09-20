package config

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestSetScalarPreservesCommentsAndOrder(t *testing.T) {
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

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	got := string(data)
	for _, comment := range []string{
		"# top comment",
		"# inline key comment",
		"# image section comment",
		"# model comment",
	} {
		if !strings.Contains(got, comment) {
			t.Errorf("comment %q did not survive:\n%s", comment, got)
		}
	}
	if !strings.Contains(got, "model: gpt-image-2") {
		t.Errorf("new value missing:\n%s", got)
	}
	if !strings.Contains(got, "size: '1024x768'") {
		t.Errorf("unrelated scalar style changed:\n%s", got)
	}
	providerAt := strings.Index(got, "provider: agnes")
	modelAt := strings.Index(got, "model: gpt-image-2")
	sizeAt := strings.Index(got, "size: '1024x768'")
	if providerAt < 0 || modelAt < 0 || sizeAt < 0 || providerAt > modelAt || modelAt > sizeAt {
		t.Errorf("key order changed:\n%s", got)
	}
}

func TestSetScalarKeepsTypes(t *testing.T) {
	cases := []struct {
		name     string
		key      string
		value    string
		wantTag  string
		wantVal  string
		wantText string
	}{
		{"int stays int", "defaults.chat.max_iterations", "20", "!!int", "20", "max_iterations: 20"},
		{"bool stays bool", "defaults.chat.allow_tool_override", "true", "!!bool", "true", "allow_tool_override: true"},
		{"float stays float", "defaults.chat.temperature", "0.8", "!!float", "0.8", "temperature: 0.8"},
		{"string stays string", "defaults.image.provider", "openai", "!!str", "openai", "provider: openai"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFixture(t, "config.yaml", nodeFixture)
			doc, err := LoadNode(path)
			if err != nil {
				t.Fatalf("LoadNode() error = %v", err)
			}
			if err := SetScalar(doc, tc.key, tc.value); err != nil {
				t.Fatalf("SetScalar() error = %v", err)
			}
			node, err := NodeAt(doc, tc.key)
			if err != nil {
				t.Fatalf("NodeAt() error = %v", err)
			}
			if node.Tag != tc.wantTag || node.Value != tc.wantVal {
				t.Errorf("node = tag %s value %q, want tag %s value %q", node.Tag, node.Value, tc.wantTag, tc.wantVal)
			}
			if err := SaveNode(path, doc); err != nil {
				t.Fatalf("SaveNode() error = %v", err)
			}
			data, _ := os.ReadFile(path)
			if !strings.Contains(string(data), tc.wantText) {
				t.Errorf("file does not contain %q:\n%s", tc.wantText, data)
			}
		})
	}
}

func TestSetScalarRejectsWrongConversion(t *testing.T) {
	path := writeFixture(t, "config.yaml", nodeFixture)
	doc, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode() error = %v", err)
	}
	err = SetScalar(doc, "defaults.chat.max_iterations", "many")
	if err == nil || !strings.Contains(err.Error(), "cannot convert") {
		t.Errorf("SetScalar() error = %v, want conversion error", err)
	}
}

func TestSetScalarMissingParent(t *testing.T) {
	path := writeFixture(t, "config.yaml", nodeFixture)
	doc, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode() error = %v", err)
	}
	err = SetScalar(doc, "defaults.video.model", "veo-3")
	if !errors.Is(err, ErrSectionNotFound) {
		t.Errorf("SetScalar() error = %v, want ErrSectionNotFound", err)
	}
}

func TestSetScalarCreatesLeafOnly(t *testing.T) {
	path := writeFixture(t, "config.yaml", nodeFixture)
	doc, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode() error = %v", err)
	}
	if err := SetScalar(doc, "defaults.chat.batch_size", "20"); err != nil {
		t.Fatalf("SetScalar(int leaf) error = %v", err)
	}
	if err := SetScalar(doc, "defaults.chat.model", "deepseek-v4-flash"); err != nil {
		t.Fatalf("SetScalar(string leaf) error = %v", err)
	}

	intNode, err := NodeAt(doc, "defaults.chat.batch_size")
	if err != nil {
		t.Fatalf("NodeAt() error = %v", err)
	}
	if intNode.Tag != "!!int" {
		t.Errorf("new numeric leaf tag = %s, want !!int", intNode.Tag)
	}
	strNode, err := NodeAt(doc, "defaults.chat.model")
	if err != nil {
		t.Fatalf("NodeAt() error = %v", err)
	}
	if strNode.Tag != "!!str" {
		t.Errorf("new string leaf tag = %s, want !!str", strNode.Tag)
	}
}

func TestSetScalarEmptyDocument(t *testing.T) {
	path := writeFixture(t, "config.yaml", "")
	doc, err := LoadNode(path)
	if err != nil {
		t.Fatalf("LoadNode() error = %v", err)
	}
	if _, err := NodeAt(doc, "api_key"); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("NodeAt(empty doc) error = %v, want ErrKeyNotFound", err)
	}
	if err := SetScalar(doc, "api_key", "sk-new"); err != nil {
		t.Fatalf("SetScalar() error = %v", err)
	}
	if err := SaveNode(path, doc); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "api_key: sk-new") {
		t.Errorf("file = %q, want api_key key", data)
	}

	doc, _ = LoadNode(path)
	if err := SetScalar(doc, "defaults.image.model", "x"); !errors.Is(err, ErrSectionNotFound) {
		t.Errorf("SetScalar(nested, empty doc) error = %v, want ErrSectionNotFound", err)
	}
}
