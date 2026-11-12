package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandTreeContract(t *testing.T) {
	if !rootCmd.SilenceErrors || !rootCmd.SilenceUsage {
		t.Fatal("root command must silence errors and usage (subcommands inherit both)")
	}

	seen := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		name := c.Name()
		if name == "" {
			t.Errorf("%q has an empty command name", c.Use)
		}
		full := c.CommandPath()
		if seen[full] {
			t.Errorf("duplicate command path %q", full)
		}
		seen[full] = true
		if c.RunE != nil || c.Run != nil {
			if strings.TrimSpace(c.Short) == "" {
				t.Errorf("command %q has no Short description", name)
			}
			if c.Flags().Lookup("help") != nil {
				t.Errorf("command %q must not define a --help flag (cobra owns it)", name)
			}
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(rootCmd)
}

func TestAgentHelpFooterOnEveryRunnableCommand(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c != rootCmd && (c.RunE != nil || c.Run != nil) && !helpFooterExempt[c.Name()] {
			if footer := agentHelpFooter(c); footer == "" {
				t.Errorf("command %q has no agent help footer", c.Name())
			}
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(rootCmd)
}
