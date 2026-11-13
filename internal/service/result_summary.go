package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/ui"
)

// ResultSummary is the self-describing footer printed after a successful
// generate/transform command. It intentionally uses plain text so agents that
// drive the CLI through a shell can read it without a JSON contract.
type ResultSummary struct {
	Files    []string
	Model    string
	Provider string
	TaskID   string
	Cost     float64
	HasCost  bool
	Next     string
}

// PrintResultSummary writes the footer. Empty optional fields are omitted, so a
// synchronous local command prints only Files/Model/Next.
func PrintResultSummary(s ResultSummary) {
	if text := ResultSummaryText(s); text != "" {
		ui.Print(text)
	}
}

// ResultSummaryText renders the same footer as PrintResultSummary. MCP uses it
// to embed the footer in a tool result instead of writing to stdout.
func ResultSummaryText(s ResultSummary) string {
	if len(s.Files) == 0 && s.TaskID == "" {
		return ""
	}
	var b strings.Builder
	if len(s.Files) > 0 {
		b.WriteString("Saved:\n")
		for _, f := range s.Files {
			fmt.Fprintf(&b, "  %s\n", absolutePath(f))
		}
	}
	if s.Model != "" {
		if s.Provider != "" {
			fmt.Fprintf(&b, "Model: %s (%s)\n", s.Model, strings.ToLower(s.Provider))
		} else {
			fmt.Fprintf(&b, "Model: %s\n", s.Model)
		}
	}
	if s.TaskID != "" {
		fmt.Fprintf(&b, "Task: %s\n", s.TaskID)
	}
	if s.HasCost && s.Cost > 0 {
		fmt.Fprintf(&b, "Cost: $%.4f\n", s.Cost)
	}
	next := s.Next
	if next == "" && len(s.Files) > 0 {
		next = fmt.Sprintf("aigc-cli preview %s", absolutePath(s.Files[0]))
	}
	if next != "" {
		fmt.Fprintf(&b, "Next: %s\n", next)
	}
	return b.String()
}

// absolutePath returns an absolute path when possible. Footer paths must be
// unambiguous for agents that may run the next command from another directory.
func absolutePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
