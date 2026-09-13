// Package models implements the `aigc-cli models` command.
package models

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

// Deps carries the runtime configuration the models command needs.
type Deps struct {
	APIBase         string
	ResolveProvider func(string) *provider.EffectiveProvider
}

var d Deps

var (
	modelType string
	priceArg  string
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

Marketplace flags (use the APIMart-compatible marketplace API at the
configured base URL):
  --type, -t <type>      Filter by media type: image, video, chat
  --price, -p [model]    Show pricing, or specify a model name for details

Examples:
  aigc-cli models
  aigc-cli models --type image
  aigc-cli models --price
  aigc-cli models --price gpt-4o`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d = deps()
			return runModels(cmd, args)
		},
	}
	cmd.Flags().StringVarP(&modelType, "type", "t", "", "Filter by media type (APIMart marketplace): image, video, chat")
	cmd.Flags().StringVarP(&priceArg, "price", "p", "", "Show pricing column (no arg) or model pricing details (with model name) (APIMart only)")
	return cmd
}

func runModels(cmd *cobra.Command, args []string) error {
	p := d.ResolveProvider("models")
	isOpenRouter := p.ProviderType == provider.OpenRouter

	mediaType := ""
	if modelType != "" {
		mediaType = modelType
	} else if len(args) > 0 {
		mediaType = args[0]
	}

	priceChanged := cmd.Flags().Changed("price")
	if priceChanged && priceArg != "" {
		return runModelsPricing(priceArg)
	}

	if priceChanged || modelType != "" {
		cmdPriceChanged = priceChanged
		if isOpenRouter && knownMediaTypes[mediaType] {
			return runModelsOpenRouterDiscovery(mediaType)
		}
		return runModelsMarketplace(mediaType)
	}

	if len(args) > 0 {
		if knownMediaTypes[args[0]] {
			if isOpenRouter {
				return runModelsOpenRouterDiscovery(args[0])
			}
			return runModelsMarketplace(args[0])
		}
		return runModelsDetail(args[0], p)
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
