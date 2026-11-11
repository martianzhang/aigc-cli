package redact

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

func newInitCmd() *cobra.Command {
	var modelID string
	var list bool
	var force bool
	var modelsRoot string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Show setup and install the optional local ONNX model",
		Example: `  # Show the offline rules, profile and model setup
  aigc-cli redact init

  # List downloadable local ONNX models
  aigc-cli redact init --list

  # Install the privyscope zh-Hans token classifier
  aigc-cli redact init --model zh-pii

  # Re-download and overwrite an existing install
  aigc-cli redact init --model zh-pii --force`,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			loadDeps()
			switch {
			case list:
				return writeTokenclsModelsTable(cmd.OutOrStdout())
			case modelID != "":
				return installTokenclsModel(cmd, modelID, modelsRoot, force)
			default:
				writeSetup(cmd.OutOrStdout())
				return nil
			}
		},
	}
	cmd.Flags().StringVar(&modelID, "model", "", "Install a local ONNX tokencls model (e.g. zh-pii)")
	cmd.Flags().BoolVar(&list, "list", false, "List downloadable tokencls models")
	cmd.Flags().BoolVar(&force, "force", false, "Re-download even if a file already exists")
	cmd.Flags().StringVar(&modelsRoot, "models-dir", "", "Models root (default ~/.config/aigc-cli/models)")
	return cmd
}

func writeSetup(out io.Writer) {
	fmt.Fprintf(out, "redact works offline with %d built-in rules; no model download is required.\n\n", len(redactlib.BuiltinRuleNames()))
	fmt.Fprintln(out, "Optional model engine (better recall for organizations, aliases and addresses):")
	fmt.Fprintln(out, "  1. Run a local model with Ollama (recommended) or any OpenAI-compatible server.")
	fmt.Fprintln(out, "  2. Import HaS 4.0: ollama create has4 -f Modelfile   (FROM ./has_4.0_0.6B_q4.gguf)")
	fmt.Fprintln(out, "  3. Configure:")
	fmt.Fprintln(out, "       providers.ollama.type: ollama")
	fmt.Fprintln(out, "       providers.ollama.base_url: http://127.0.0.1:11434/v1")
	fmt.Fprintln(out, "       defaults.redact.provider: ollama")
	fmt.Fprintln(out, "       defaults.redact.model: has4")
	fmt.Fprintln(out, "  4. Use: aigc-cli redact scan --engine auto file.txt")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Profiles (keep rule sets in standalone files):")
	fmt.Fprintln(out, "  defaults.redact.profile: strict")
	fmt.Fprintln(out, "  defaults.redact.profiles:")
	fmt.Fprintln(out, "    strict: ./redact/strict.yaml")
	fmt.Fprintln(out, "  aigc-cli redact scan file.txt --profile strict")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Local ONNX token-classification model (optional, offline):")
	fmt.Fprintln(out, "  aigc-cli redact init --list")
	fmt.Fprintln(out, "  aigc-cli redact init --model zh-pii")
	fmt.Fprintln(out, "  Then set defaults.redact.model_engine: tokencls.")
	fmt.Fprintln(out, "  Model: privyscope zh-Hans, Apache-2.0")
	fmt.Fprintln(out, "  Source:   https://github.com/zafrem/privyscope-zh-hans")
	fmt.Fprintln(out, "  Download: https://github.com/martianzhang/aigc-cli-models/releases/tag/v1")
}
