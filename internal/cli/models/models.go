// Package models implements the `aigc-cli models` command.
package models

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// Deps carries the runtime configuration the models command needs.
type Deps struct {
	APIBase         string
	Model           string
	ResolveProvider func(string) *provider.EffectiveProvider
	Providers       map[string]*types.NamedProvider
}

var d Deps

var (
	modelType           string
	priceArg            string
	modelsDryRun        bool
	modelsListProviders bool
)

// priceListValue is the sentinel pflag stores for a bare `--price` (the flag
// takes an optional model name).
const priceListValue = "__list__"

var knownMediaTypes = func() map[string]bool {
	out := make(map[string]bool, len(openRouterModalities))
	for k := range openRouterModalities {
		out[k] = true
	}
	return out
}()

// marketplaceTypes are the media types the APIMart marketplace API understands.
var marketplaceTypes = map[string]bool{"image": true, "video": true, "chat": true}

// NewCommand builds the `models` command. deps is resolved at run time.
func NewCommand(deps func() Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "models [--type <type>] [--price [model-name]]",
		Aliases:      []string{"model"},
		Short:        "List available AI models (also: model)",
		SilenceUsage: true,
		Long: `List models from any OpenAI-compatible API.

Without flags: queries the /v1/models endpoint (works with OpenAI,
OpenRouter, or any OpenAI-compatible relay).

Model detail: pass the model name as an argument or via --model/-m.
  aigc-cli models gpt-4o
  aigc-cli models --model gpt-4o

Type filter:
  --type, -t <type>      Filter by model type. OpenRouter supports every
                         output modality: text (chat), image, video, audio,
                         speech (tts), transcription (stt), embeddings,
                         rerank, decisions, all. APIMart supports image,
                         video and chat.

Pricing:
  --price [model]        Show the pricing column, or one model's pricing.
                         APIMart uses its pricing API; OpenRouter prints the
                         model record (which includes pricing).

Provider list:
  --list-providers       List configured providers (name, type, host and
                         whether a key is set) without calling any API

When no API key is resolved, the command prints the configured providers as
candidates instead of failing with a bare error.

Use --dry-run to print the equivalent curl request without calling the API.

Examples:
  aigc-cli models
  aigc-cli models --list-providers
  aigc-cli models --provider apimart
  aigc-cli models --type image
  aigc-cli models --provider openrouter --type speech
  aigc-cli models --provider openrouter --type all
  aigc-cli models --price
  aigc-cli models --price gpt-image-2-official
  aigc-cli models --model gpt-4o
  aigc-cli models gpt-4o --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d = deps()
			return runModels(cmd, args)
		},
	}
	cmd.Flags().StringVarP(&modelType, "type", "t", "", "Filter by model type (OpenRouter: text, image, video, audio, speech, transcription, embeddings, rerank, decisions, all)")
	cmd.Flags().StringVar(&priceArg, "price", "", "Show pricing column (no arg) or model pricing details (with model name)")
	cmd.Flags().Lookup("price").NoOptDefVal = priceListValue
	cmd.Flags().BoolVar(&modelsDryRun, "dry-run", false, "Print the equivalent curl request without calling the API")
	cmd.Flags().BoolVar(&modelsListProviders, "list-providers", false, "List configured providers without calling any API")
	return cmd
}

func runModels(cmd *cobra.Command, args []string) error {
	if modelsListProviders {
		return runListProviders()
	}

	p := d.ResolveProvider("models")
	isOpenRouter := p.ProviderType == provider.OpenRouter

	// The --type flag wins; otherwise a positional value may name a type.
	mediaType := modelType
	if mediaType == "" && len(args) > 0 && knownMediaTypes[args[0]] {
		mediaType = args[0]
	}

	// A positional value that is not a type is a model name. When there is no
	// positional argument, fall back to the global --model/-m flag so
	// `models -m <name>` behaves like `models <name>`.
	modelName := ""
	switch {
	case len(args) > 0 && !knownMediaTypes[args[0]]:
		modelName = args[0]
	case len(args) == 0:
		modelName = d.Model
	}

	priceChanged := cmd.Flags().Changed("price")
	cmdPriceChanged = priceChanged
	if priceChanged {
		priceModel := priceArg
		if priceModel == priceListValue {
			priceModel = ""
		}
		// The price flag's value is optional, so `--price <model>` leaves the
		// model name as a positional argument instead of the flag value.
		if priceModel == "" {
			priceModel = modelName
		}
		if priceModel != "" {
			if isOpenRouter {
				// OpenRouter's /model/{author}/{slug} record carries pricing
				// for every modality; the APIMart pricing API does not know
				// these models.
				return runModelsDetail(priceModel, p)
			}
			return runModelsPricing(priceModel)
		}
		if isOpenRouter {
			return runModelsOpenRouterDiscovery(p, mediaType)
		}
		return runModelsMarketplace(mediaType)
	}

	if modelType != "" {
		if isOpenRouter {
			return runModelsOpenRouterDiscovery(p, mediaType)
		}
		return runModelsMarketplace(mediaType)
	}

	// Both the plain listing and single-model detail call /v1/models, which
	// needs a key. Print the configured providers as candidates instead of a
	// bare error so the user can pick one immediately.
	if (modelName != "" || len(args) == 0) && missingKey(p) {
		return noKeyFallback(p)
	}

	if modelName != "" {
		return runModelsDetail(modelName, p)
	}

	if mediaType != "" {
		if isOpenRouter {
			return runModelsOpenRouterDiscovery(p, mediaType)
		}
		return runModelsMarketplace(mediaType)
	}

	return runModelsOpenAI(p)
}

// isHTML checks whether the first non-whitespace bytes look like HTML.
func isHTML(body []byte) bool {
	for _, b := range body {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '<':
			return true
		default:
			return false
		}
	}
	return false
}

var cmdPriceChanged bool

// printAPIURL prints the API endpoint being called in a consistent format.
func printAPIURL(apiURL string) {
	fmt.Printf("API: %s\n", apiURL)
}
