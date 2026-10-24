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

var knownMediaTypes = map[string]bool{"image": true, "video": true, "chat": true}

// NewCommand builds the `models` command. deps is resolved at run time.
func NewCommand(deps func() Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "models [--type image|video|chat] [--price [model-name]]",
		Aliases:      []string{"model"},
		Short:        "List available AI models (also: model)",
		SilenceUsage: true,
		Long: `List models from any OpenAI-compatible API.

Without flags: queries the /v1/models endpoint (works with OpenAI,
OpenRouter, or any OpenAI-compatible relay).

Model detail: pass the model name as an argument or via --model/-m.
  aigc-cli models gpt-4o
  aigc-cli models --model gpt-4o

Marketplace flags (use the APIMart-compatible marketplace API at the
configured base URL):
  --type, -t <type>      Filter by media type: image, video, chat
  --price [model]        Show pricing, or specify a model name for details

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
  aigc-cli models --price
  aigc-cli models --price gpt-4o
  aigc-cli models --model gpt-4o
  aigc-cli models gpt-4o --dry-run`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d = deps()
			return runModels(cmd, args)
		},
	}
	cmd.Flags().StringVarP(&modelType, "type", "t", "", "Filter by media type (APIMart marketplace): image, video, chat")
	cmd.Flags().StringVar(&priceArg, "price", "", "Show pricing column (no arg) or model pricing details (with model name) (APIMart only)")
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

	mediaType := ""
	if modelType != "" {
		mediaType = modelType
	} else if len(args) > 0 {
		mediaType = args[0]
	}

	// A non-media-type positional argument is a model name. When none is
	// given, fall back to the global --model/-m flag so `models -m <name>`
	// behaves like `models <name>`.
	modelName := ""
	if len(args) > 0 && !knownMediaTypes[args[0]] {
		modelName = args[0]
	} else if len(args) == 0 {
		modelName = d.Model
	}

	priceChanged := cmd.Flags().Changed("price")
	if priceChanged && priceArg != "" {
		return runModelsPricing(priceArg)
	}

	if priceChanged || modelType != "" {
		cmdPriceChanged = priceChanged
		if isOpenRouter && knownMediaTypes[mediaType] {
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

	if len(args) > 0 {
		if isOpenRouter {
			return runModelsOpenRouterDiscovery(p, args[0])
		}
		return runModelsMarketplace(args[0])
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
