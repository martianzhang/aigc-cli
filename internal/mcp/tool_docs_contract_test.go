package mcp

import (
	"strings"
	"testing"
)

func TestToolDocsCoverRegistry(t *testing.T) {
	for _, info := range toolRegistry {
		doc, ok := toolDocs[info.name]
		if !ok {
			t.Errorf("%s has no toolDocs entry", info.name)
			continue
		}
		for label, value := range map[string]string{
			"When to use:": doc.When,
			"Requires:":    doc.Requires,
			"Cost:":        doc.Cost,
			"Returns:":     doc.Returns,
			"Example:":     doc.Example,
		} {
			if strings.TrimSpace(value) == "" {
				t.Errorf("%s: empty %s", info.name, label)
			}
		}
	}
}

func TestDescribeToolAppendsContract(t *testing.T) {
	got := describeTool("generate_image", "Generate images via AI")
	for _, want := range []string{
		"Generate images via AI",
		"When to use:",
		"Requires:",
		"Cost:",
		"Returns:",
		"Example:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describeTool() missing %q:\n%s", want, got)
		}
	}
}
