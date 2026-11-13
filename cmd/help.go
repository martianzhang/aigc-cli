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

// commandHelpMeta is the H1 contract for the commands agents use most. The
// values are deliberately concrete: every recovery line is a command an agent
// can copy without reading the guides.
type commandHelpMeta struct {
	Requires string
	Cost     string
	Output   string
	Async    string
	Recovery string
}

var topLevelHelpMeta = map[string]commandHelpMeta{
	"image": {
		Requires: "--model (discover: aigc-cli models --type image --provider <name>); API key unless local",
		Cost:     "paid per image; --dry-run is free",
		Output:   "files in -o (default .); absolute paths printed",
		Async:    "APIMart/ModelScope poll automatically; task id printed for 'aigc-cli task <id>'",
		Recovery: "no model -> aigc-cli models --type image --provider <name>; no key -> export OPENAI_API_KEY",
	},
	"video": {
		Requires: "--model (discover: aigc-cli models --type video --provider <name>); API key unless local",
		Cost:     "paid per video; --dry-run is free",
		Output:   "files in -o (default .); absolute paths printed",
		Async:    "async providers poll automatically; task/job id printed for recovery",
		Recovery: "no model -> aigc-cli models --type video --provider <name>; OpenRouter resume -> --job-id <id>",
	},
	"chat": {
		Requires: "API key unless Ollama/local; --model optional",
		Cost:     "paid per request; --dry-run is free",
		Output:   "answer on stdout; --save writes JSONL sessions",
		Recovery: "no input -> pass -p/--message or pipe text; no key -> aigc-cli --print-config",
	},
	"music": {
		Requires: "--model for the backend (suno/flowmusic/Lyria-3/Fun-Music); API key",
		Cost:     "paid per track; --dry-run is free",
		Output:   "audio files in -o (default .); absolute paths printed",
		Async:    "APIMart polls automatically; task id printed for 'aigc-cli task <id>'",
		Recovery: "no model -> aigc-cli music generate --help; task status -> aigc-cli task <id>",
	},
	"audio": {
		Requires: "cloud: API key + model; local: one-time 'aigc-cli audio init'",
		Cost:     "cloud TTS/STT is paid; local inference is free; --dry-run is free",
		Output:   "audio/transcript files in -o (default .); absolute paths printed",
		Recovery: "missing local model -> aigc-cli audio init; no key -> aigc-cli --print-config",
	},
	"upscale": {
		Requires: "one-time 'aigc-cli upscale init' (ONNX model + runtime)",
		Cost:     "free (local inference)",
		Output:   "PNG in -o (default .); absolute paths printed",
		Recovery: "missing model -> aigc-cli upscale init --model <id>",
	},
	"ocr": {
		Requires: "one-time 'aigc-cli ocr init' for offline mode; online mode needs a provider",
		Cost:     "offline inference is free; online parsing may be paid",
		Output:   "text on stdout or files in -o; absolute paths printed",
		Recovery: "missing model -> aigc-cli ocr init; online errors -> aigc-cli --print-config",
	},
	"redact": {
		Requires: "no model required for built-in rules; optional 'aigc-cli redact init'",
		Cost:     "free for rules; optional model/provider calls may be paid",
		Output:   "redacted file or stdout; mapping file with --mapping",
		Recovery: "residual check -> aigc-cli redact check <file> --mapping <map>; rules -> aigc-cli redact rules",
	},
	"detect": {
		Requires: "optional ONNX model: 'aigc-cli detect init'; metadata/FFT checks work without it",
		Cost:     "free (offline)",
		Output:   "signal report on stdout; --json not required for agents",
		Recovery: "missing model -> aigc-cli detect init; watermark learning -> see --help examples",
	},
	"mcp": {
		Requires: "stdio transport; provider/model requirements depend on the tools you enable",
		Cost:     "varies by tool; generation tools may be paid",
		Output:   "JSON-RPC on stdout; progress goes to stderr",
		Async:    "generation tools may poll; get_task returns task/job status",
		Recovery: "list tools -> aigc-cli mcp --list-tools; config -> aigc-cli --print-config",
	},
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
	if meta, ok := topLevelHelpMeta[topLevelCommand(c)]; ok {
		b.WriteString("\nCommand contract:\n")
		writeMetaLine := func(label, value string) {
			if value != "" {
				fmt.Fprintf(&b, "  %-11s %s\n", label+":", value)
			}
		}
		writeMetaLine("Requires", meta.Requires)
		writeMetaLine("Cost", meta.Cost)
		writeMetaLine("Output", meta.Output)
		writeMetaLine("Async", meta.Async)
		writeMetaLine("Recovery", meta.Recovery)
	}
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
