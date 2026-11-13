package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestTopLevelHelpContract enforces the H1 contract for the commands agents use
// most: each help page must state requirements, cost, output and recovery.
func TestTopLevelHelpContract(t *testing.T) {
	for name, meta := range topLevelHelpMeta {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := rootCmd.Find([]string{name})
			if err != nil {
				t.Fatalf("Find(%s): %v", name, err)
			}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			t.Cleanup(func() {
				cmd.SetOut(os.Stdout)
				cmd.SetErr(os.Stderr)
			})
			rootCmd.HelpFunc()(cmd, nil)
			out := buf.String()

			fields := map[string]string{
				"Requires:": meta.Requires,
				"Cost:":     meta.Cost,
				"Output:":   meta.Output,
				"Recovery:": meta.Recovery,
			}
			for label, value := range fields {
				if value == "" {
					continue
				}
				if !strings.Contains(out, label) || !strings.Contains(out, value) {
					t.Errorf("%s --help missing %q / %q", name, label, value)
				}
			}
			if !strings.Contains(out, "aigc-cli "+name) {
				t.Errorf("%s --help has no runnable example", name)
			}
		})
	}
}
