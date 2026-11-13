package service

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ui"
)

func TestPrintResultSummary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "image.png")

	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	PrintResultSummary(ResultSummary{
		Files:    []string{file},
		Model:    "gpt-image-2",
		Provider: "apimart",
		TaskID:   "task_1",
		Cost:     0.08,
		HasCost:  true,
		Next:     "aigc-cli task task_1",
	})

	out := buf.String()
	for _, want := range []string{
		"Saved:",
		file,
		"Model: gpt-image-2 (apimart)",
		"Task: task_1",
		"Cost: $0.0800",
		"Next: aigc-cli task task_1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestPrintResultSummaryLocalDefaultsToPreview(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "upscaled.png")

	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	PrintResultSummary(ResultSummary{
		Files: []string{file},
		Model: "local: realesr-general-x4v3",
	})

	out := buf.String()
	if strings.Contains(out, "Cost:") {
		t.Errorf("local result must not print cost:\n%s", out)
	}
	if !strings.Contains(out, "Next: aigc-cli preview "+file) {
		t.Errorf("missing preview hint:\n%s", out)
	}
}

func TestPrintResultSummaryOmitsEmpty(t *testing.T) {
	var buf bytes.Buffer
	restore := ui.SetOut(&buf)
	defer restore()

	PrintResultSummary(ResultSummary{})
	if buf.Len() != 0 {
		t.Fatalf("empty summary wrote %q", buf.String())
	}
}
