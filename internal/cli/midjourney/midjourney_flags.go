package midjourney

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// registerTaskActionFlags registers the common flags shared by task-action subcommands
// (upscale, variation, high-variation, low-variation, inpaint).
func registerTaskActionFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&mjTaskID, "task-id", "", "Parent task ID (required)")
	f.IntVar(&mjIndex, "index", 0, "Tile index (1-4, or omit for single-image tasks)")
	f.StringVar(&mjCustomID, "custom-id", "", "Button customId (bypasses auto matching)")
	f.StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
}

// registerImagineStructuredFlags registers the shared imagine structured-field flags
// used by imagine, edits, and similar.
func registerImagineStructuredFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&mjSize, "size", "", `Aspect ratio, e.g. "16:9", "1:1", "9:16"`)
	f.StringVar(&mjQuality, "quality", "", `Quality: "0.25", "0.5", "1", "2"`)
	f.StringVar(&mjStyle, "style", "", `Style override, e.g. "raw"`)
	f.StringVar(&mjVersion, "version", "", `MJ version: "8.1", "7", "6.1", "5.2", "5.1"`)
	f.IntVar(&mjSeed, "seed", 0, "Random seed for reproducibility")
	f.StringVar(&mjNegPrompt, "negative-prompt", "", `Negative prompt (--no)`)
	f.IntVar(&mjStylize, "stylize", 0, "Stylize value (--s, 0-1000)")
	f.IntVar(&mjChaos, "chaos", 0, "Chaos value (--c, 0-100)")
	f.IntVar(&mjWeird, "weird", 0, "Weird value (--w, 0-3000)")
	f.BoolVar(&mjTile, "tile", false, "Tile mode (--tile)")
	f.BoolVar(&mjNiji, "niji", false, "Niji model switch")
	f.Float64Var(&mjIw, "iw", 0, "Image weight (--iw, 0-3)")
	f.IntVar(&mjCw, "cw", 0, "Reference weight for character ref (--cw, 0-100)")
	f.IntVar(&mjSw, "sw", 0, "Style weight (--sw, 0-1000)")
	f.StringVar(&mjCref, "cref", "", "Character reference image URL (--cref)")
	f.StringVar(&mjSref, "sref", "", "Style reference image URL (--sref)")
	f.StringVar(&mjDref, "dref", "", "Depth reference image URL (--dref)")
	f.Float64Var(&mjDw, "dw", 0, "Depth weight (--dw, 0-100)")
	f.IntVar(&mjRepeat, "repeat", 0, "Repeat count (--repeat, 2-40)")
	f.BoolVar(&mjRaw, "raw", false, "Raw style (--raw, v5.1+)")
	f.BoolVar(&mjDraft, "draft", false, "Draft mode (--draft, v7+)")
	f.BoolVar(&mjHd, "hd", false, "HD mode (--hd, v8/v8.1)")
	f.IntVar(&mjStop, "stop", 0, "Early stop (--stop, 10-100)")
	f.StringVar(&mjExtra, "extra", "", "Extra flags appended verbatim (--xxx)")
}

// registerSharedFlags registers flags common to all MJ subcommands.
func registerSharedFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&mjPrompt, "prompt", "p", "", "Text prompt (or \"-\" for stdin)")
	f.StringArrayVar(&mjImageURLs, "image-url", nil, "Image URL or local path (repeatable)")
	f.StringVar(&mjTaskID, "task-id", "", "Parent task ID (required for follow-up actions)")
	f.IntVar(&mjIndex, "index", 0, "Tile index (1-4)")
	f.StringVar(&mjCustomID, "custom-id", "", "Button customId for direct action")
	f.StringVar(&mjSpeed, "speed", "", "Speed: relax (default), fast, turbo")
	f.BoolVar(&mjDryRun, "dry-run", false, "Print request parameters without calling API")
	f.StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
}

// ============================================================================
// Prompt resolver
// ============================================================================

type mjTaskActionSpec struct {
	name    string
	short   string
	long    string
	example string
	action  string
}

// registerMJTaskActionSubcommand creates a task-action subcommand (upscale, variation, etc.).
func registerMJTaskActionSubcommand(spec mjTaskActionSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:     spec.name,
		Short:   spec.short,
		Long:    spec.long,
		Example: spec.example,
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := buildMJTaskActionReqFromJSON(cmd)
			if err != nil {
				return err
			}
			// Merge config defaults
			if cfg, err := config.LoadDefaults(options.Shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil && cfg.Defaults.Midjourney != nil {
				// Only speed is relevant for task actions
				if req.Speed == "" && cfg.Defaults.Midjourney.Speed != "" {
					req.Speed = cfg.Defaults.Midjourney.Speed
				}
			}
			c := NewClient()
			return runMJSubmitAndPoll(c, spec.action, req)
		},
	}
	registerTaskActionFlags(cmd)
	cmd.Flags().StringVar(&mjJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	return cmd
}

func resolveMJPrompt(cmd *cobra.Command) (string, error) {
	input := mjPrompt
	if input == "" && cmd.Flags().Changed("prompt") {
		// prompt was set to empty explicitly -- that's OK for some endpoints
		return "", nil
	}
	if input == "" {
		// For imagine/prompt-required commands, we'll check later
		return "", nil
	}
	if input == "-" || service.IsFile(input) {
		data, err := service.ReadInput(input)
		if err != nil {
			return "", fmt.Errorf("failed to read prompt: %w", err)
		}
		return string(data), nil
	}
	return input, nil
}

// ============================================================================
// Curl builder
// ============================================================================
