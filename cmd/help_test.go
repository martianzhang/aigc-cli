package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTopLevelCommand(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	top := &cobra.Command{Use: "top"}
	leaf := &cobra.Command{Use: "leaf"}
	root.AddCommand(top)
	top.AddCommand(leaf)

	tests := []struct {
		name string
		cmd  *cobra.Command
		want string
	}{
		{"direct child of root", top, "top"},
		{"nested leaf", leaf, "top"},
		{"root itself", root, "root"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := topLevelCommand(tc.cmd); got != tc.want {
				t.Errorf("topLevelCommand() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHasFlag(t *testing.T) {
	c := &cobra.Command{Use: "c"}
	c.Flags().Bool("dry-run", false, "")
	if !hasFlag(c, "dry-run") {
		t.Fatal("hasFlag(dry-run) = false, want true")
	}
	if hasFlag(c, "missing") {
		t.Fatal("hasFlag(missing) = true, want false")
	}
}

func TestAgentHelpFooter(t *testing.T) {
	tests := []struct {
		name     string
		build    func() *cobra.Command
		want     []string
		wantNone []string
	}{
		{
			name: "online async command with dry-run and json",
			build: func() *cobra.Command {
				c := &cobra.Command{Use: "image"}
				c.Flags().Bool("dry-run", false, "")
				c.Flags().String("json", "", "")
				return c
			},
			want: []string{
				"export OPENAI_API_KEY=",
				"Precedence:",
				"Preview:",
				"Async:",
				"JSON input:",
				"Exit:",
			},
			wantNone: []string{"Offline:"},
		},
		{
			name:     "offline command",
			build:    func() *cobra.Command { return &cobra.Command{Use: "detect"} },
			want:     []string{"Offline:     first run needs models: aigc-cli detect init"},
			wantNone: []string{"Preview:", "Async:", "JSON input:"},
		},
		{
			name:     "root command is skipped",
			build:    func() *cobra.Command { return rootCmd },
			wantNone: []string{"Running this command"},
		},
		{
			name:     "utility command is skipped",
			build:    func() *cobra.Command { return &cobra.Command{Use: "completion"} },
			wantNone: []string{"Running this command"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := agentHelpFooter(tc.build())
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("footer missing %q:\n%s", want, got)
				}
			}
			for _, nope := range tc.wantNone {
				if strings.Contains(got, nope) {
					t.Errorf("footer unexpectedly contains %q:\n%s", nope, got)
				}
			}
		})
	}
}

// TestInstallAgentHelpAppendsFooter ensures the shared footer reaches the help
// output of a subcommand through the wrapped help function.
func TestInstallAgentHelpAppendsFooter(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	sub := &cobra.Command{Use: "sub", Short: "a subcommand"}
	root.AddCommand(sub)

	installAgentHelp(root)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	if err := sub.Help(); err != nil {
		t.Fatalf("Help() error: %v", err)
	}
	if !strings.Contains(buf.String(), "Running this command") {
		t.Errorf("help output missing agent footer:\n%s", buf.String())
	}
}
