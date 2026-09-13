package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

var (
	modelType string
	priceArg  string // "" = not set, "--price" bare = list with pricing, "--price <model>" = detail
)

// modelsCmd represents the `aigc-cli models` command.
var modelsCmd = &cobra.Command{
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
	RunE: runModels,
}

// knownMediaTypes are the valid marketplace type filters.
var knownMediaTypes = map[string]bool{"image": true, "video": true, "chat": true}

func runModels(cmd *cobra.Command, args []string) error {
	p := shared.ResolveProvider(ProviderNameModels)
	isOpenRouter := p.ProviderType == provider.OpenRouter

	// --type flag overrides positional arg
	mediaType := ""
	if modelType != "" {
		mediaType = modelType
	} else if len(args) > 0 {
		mediaType = args[0]
	}

	// --price with a model name → APIMart pricing detail
	priceChanged := cmd.Flags().Changed("price")
	if priceChanged && priceArg != "" {
		return runModelsPricing(priceArg)
	}

	// --type or --price (bare) → marketplace or OpenRouter discovery
	if priceChanged || modelType != "" {
		cmdPriceChanged = priceChanged
		if isOpenRouter && knownMediaTypes[mediaType] {
			return runModelsOpenRouterDiscovery(mediaType)
		}
		return runModelsMarketplace(mediaType)
	}

	// Positional arg alone: known media type → marketplace or OpenRouter, else → /v1/models/{model}
	if len(args) > 0 {
		if knownMediaTypes[args[0]] {
			if isOpenRouter {
				return runModelsOpenRouterDiscovery(args[0])
			}
			return runModelsMarketplace(args[0])
		}
		return runModelsDetail(args[0], p)
	}

	// No args, no flags → universal /v1/models
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

// cmdPriceChanged is set by runModels before it dispatches to marketplace,
// so that runModelsMarketplace can check it without needing a cmd param.
var cmdPriceChanged bool

func init() {
	modelsCmd.Flags().StringVarP(&modelType, "type", "t", "", "Filter by media type (APIMart marketplace): image, video, chat")
	modelsCmd.Flags().StringVarP(&priceArg, "price", "p", "", "Show pricing column (no arg) or model pricing details (with model name) (APIMart only)")
	rootCmd.AddCommand(modelsCmd)
}

// printAPIURL prints the API endpoint being called in a consistent format.
func printAPIURL(apiURL string) {
	fmt.Printf("API: %s\n", apiURL)
}
