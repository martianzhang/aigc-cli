// Package decision implements the `decision` command: typed decision-model
// questions (choice / noul / score) evaluated against a state block via the
// System One (TypeSafe Jev) API served by Ollama, OpenRouter and relays.
package decision

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

var (
	decisionQuestions   []string
	decisionState       string
	decisionImages      []string
	decisionImageResize int
	decisionList        bool
	decisionDryRun      bool
)

var decisionCmd = &cobra.Command{
	Use:          "decision",
	Aliases:      []string{"decide", "systemone"},
	Short:        "Run typed decision questions (choice / yes-no / score) against text",
	SilenceUsage: true,
	Long: `Run typed "decision" questions (single-choice / yes-no / rating) against a block
of text and get structured answers with calibrated probabilities. This command
does not generate free-form text.

The question bank is JSON or JSONC, passed via --json <path|inline-json|-> or
configured as defaults.decision.bank: a "questions" object of 1-64 named
questions (choice / noul / score), plus optional default "state" and "model".

Decision models (Clef / Clef-Flash) can also judge images supplied with
--image: each image (raw base64) is shared by all questions and scored jointly
with the text state. Images are downscaled to --image-resize (default 1024px
longest edge) because pixel dimensions, not file size, drive vision-token cost
and decision latency.

Supported providers: Ollama (local, tev1/nimble, no API key), OpenRouter
(typesafe/jev-latest), TypeSafe AI, LLM Gateway and LiteLLM proxies. Endpoint
paths differ per provider and are normalized automatically.`,
	Example: `  aigc-cli decision --json docs/en/examples/decision/triage.json --questions intent --state docs/en/examples/decision/state.txt
  aigc-cli decision -P openrouter -m typesafe/jev-latest --questions intent --state state.txt
  aigc-cli decision -m clef-flash --image form.png --questions complete --state "The agent wants to submit the attached form."
  aigc-cli decision -m clef-flash --image big.png --image-resize 768 --state "Judge the attached screenshot."
  aigc-cli decision --list
  aigc-cli decision --questions intent --state state.txt --dry-run`,
	RunE: runDecision,
}

func init() {
	f := decisionCmd.Flags()
	f.StringVar(&options.Shared.JSONInput, "json", "", `Question bank (JSON/JSONC): file path, inline JSON, or "-" for stdin (falls back to defaults.decision.bank)`)
	f.StringSliceVar(&decisionQuestions, "questions", nil, "Answer only these named questions (comma-separated, repeatable; default: all)")
	f.StringVar(&decisionState, "state", "", `Material to judge: literal text, a file path, or "-" for stdin (a valid JSON object/array is sent as structured state)`)
	f.StringArrayVar(&decisionImages, "image", nil, "Attach an image (PNG/JPEG/WebP) scored jointly with the state; repeatable. Local file or base64 data URI (Clef/Clef-Flash, Ollama >= 0.35.1)")
	f.IntVar(&decisionImageResize, "image-resize", 1024, "Downscale attached images so the longest edge is at most N px before sending (0 = keep original). Larger images cost far more vision tokens and are much slower")
	f.BoolVar(&decisionList, "list", false, "List the question names available in the bank, then exit")
	f.BoolVar(&decisionDryRun, "dry-run", false, "Print the equivalent curl request without calling the API")
}

// Cmd returns the decision command.
func Cmd() *cobra.Command { return decisionCmd }

func runDecision(cmd *cobra.Command, args []string) error {
	b, err := loadBank()
	if err != nil {
		return err
	}
	if decisionList {
		fmt.Println(strings.Join(sortedQuestionNames(b.Questions), ","))
		return nil
	}

	ep := options.Shared.ResolveProvider(options.ProviderNameDecision)
	if err := options.RequireAPIKey(options.ProviderNameDecision, ep, configuredProviders()); err != nil {
		return err
	}

	questions, err := filterQuestions(b.Questions, decisionQuestions)
	if err != nil {
		return err
	}
	state, err := resolveState(decisionState, b.State)
	if err != nil {
		return err
	}
	images, err := resolveImages(decisionImages, b.Images, decisionImageResize)
	if err != nil {
		return err
	}

	cliModel := ""
	if options.HasFlagChanged(cmd, "model") {
		cliModel = options.Shared.Model
	}
	req := &provider.DecisionRequest{
		Model:     resolveModel(cliModel, b.Model, ep.Model),
		State:     state,
		Images:    images,
		Questions: questions,
	}

	if decisionDryRun {
		fmt.Println(buildDecisionCurl(req, ep))
		return nil
	}

	resp, err := provider.Decide(ep, req, decisionTimeout())
	if err != nil {
		return fmt.Errorf("decision failed: %w", err)
	}
	return printJSONResponse(resp)
}

// decisionTimeout derives the HTTP timeout: --timeout flag > 180s default.
func decisionTimeout() time.Duration {
	if options.Shared.TimeoutFlag > 0 {
		return time.Duration(options.Shared.TimeoutFlag) * time.Second
	}
	return 180 * time.Second
}

// configuredProviders returns the named providers from the loaded config.
func configuredProviders() map[string]*types.NamedProvider {
	if options.Shared.Cfg != nil {
		return options.Shared.Cfg.Providers
	}
	return nil
}
