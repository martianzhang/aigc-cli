package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// helpFooterExempt lists utility commands that don't benefit from the
// agent-oriented footer.
var helpFooterExempt = map[string]bool{
	"help":       true,
	"completion": true,
	"version":    true,
}

// localCommandRoots are top-level commands that run offline and therefore need
// a one-time model download via 'aigc-cli <command> init'.
var localCommandRoots = map[string]bool{
	"audio":         true,
	"background":    true,
	"depth":         true,
	"detect":        true,
	"ideas":         true,
	"knowledgebase": true,
	"ocr":           true,
	"upscale":       true,
	"vision":        true,
}

// asyncCommandRoots are top-level commands that submit long-running jobs and
// poll until completion, so an agent knows a task id can be re-queried.
var asyncCommandRoots = map[string]bool{
	"image":      true,
	"midjourney": true,
	"music":      true,
	"video":      true,
}

// installAgentHelp appends a compact "conventions" block to every command's
// --help output, so an agent can operate the CLI from --help alone without an
// external skill or guide. The default cobra help body is printed first.
func installAgentHelp(root *cobra.Command) {
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		defaultHelp(c, args)
		if footer := agentHelpFooter(c); footer != "" {
			fmt.Fprint(c.OutOrStdout(), footer)
		}
	})
}

// agentHelpFooter builds the per-command convention hints. It returns "" for
// the root command (which documents these conventions inline) and for utility
// commands.
func agentHelpFooter(c *cobra.Command) string {
	if c == rootCmd || helpFooterExempt[c.Name()] {
		return ""
	}

	var b strings.Builder
	b.WriteString("\nRunning this command (no external docs needed):\n")
	b.WriteString("  Setup:       export OPENAI_API_KEY=<KEY>  (or --api-key/--api-base; Ollama needs none)\n")
	b.WriteString("  Config:      aigc-cli --print-config      (effective YAML); persist: config set <key> <val> [--force]\n")
	if hasJSONInputFlag(c) {
		b.WriteString("  Precedence:  CLI flags > --json input > config.yaml > built-in defaults\n")
	} else {
		b.WriteString("  Precedence:  CLI flags > config.yaml > built-in defaults\n")
	}

	if localCommandRoots[topLevelCommand(c)] {
		fmt.Fprintf(&b, "  Offline:     first run needs models: aigc-cli %s init\n", topLevelCommand(c))
	}

	if hasFlag(c, "dry-run") {
		b.WriteString("  Preview:     --dry-run shows what would be sent without calling the API (zero cost)\n")
	}
	if asyncCommandRoots[topLevelCommand(c)] {
		b.WriteString("  Async:       long jobs poll automatically; re-query a task id with 'aigc-cli task <id>'\n")
	}
	if hasJSONInputFlag(c) {
		if c.Annotations["json_stdin_is_query"] == "true" {
			b.WriteString("  JSON input:  --json accepts a file path or inline JSON; stdin carries the query\n")
		} else {
			b.WriteString("  JSON input:  --json accepts a file path, inline JSON, or '-' for stdin\n")
		}
	}

	b.WriteString("  Files:       saved file paths are printed; use -o/--output to choose the location\n")
	b.WriteString("  Output:      human-readable text; add -v for the full request/result JSON\n")
	b.WriteString("  Exit:        0 = success, 1 = error (error message goes to stderr)\n")
	return b.String()
}

// topLevelCommand returns the direct child of the root command that owns c.
func topLevelCommand(c *cobra.Command) string {
	for c.HasParent() && c.Parent().HasParent() {
		c = c.Parent()
	}
	return c.Name()
}

// hasFlag reports whether cmd exposes a local or inherited flag by that name.
func hasFlag(c *cobra.Command, name string) bool {
	return c.Flags().Lookup(name) != nil || c.InheritedFlags().Lookup(name) != nil
}

// hasJSONInputFlag reports whether --json takes a value (a request/input
// payload). Boolean --json flags are output selectors and must not be described
// as JSON input in the agent footer.
func hasJSONInputFlag(c *cobra.Command) bool {
	f := c.Flags().Lookup("json")
	if f == nil {
		f = c.InheritedFlags().Lookup("json")
	}
	return f != nil && f.Value.Type() != "bool"
}
