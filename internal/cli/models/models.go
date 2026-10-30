// Package models implements the `aigc-cli models` command.
package models

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
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
	modelLike           string
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
  --type, -t <type>      Filter by model type. Requires a provider with a
                         catalog API: OpenRouter supports every output
                         modality (text/chat, image, video, audio, speech,
                         transcription, embeddings, rerank, decisions, all);
                         APIMart supports image, video and chat. Other
                         OpenAI-compatible providers (ModelScope, relays, …)
                         fail with a clear error — omit --type to list their
                         /v1/models instead.

Fuzzy search:
  --like <text>          Only list models whose id or name contains <text>
                         (case-insensitive). Without --type the search spans
                         every modality.

Default provider:
  OpenRouter when it is configured (its catalogs expose per-model sizing
  metadata: aspect ratios, resolution tiers, parameter ranges). Override with
  --provider or --api-base. There is no built-in API base URL: configure
  providers.<name>.base_url or the global base_url, otherwise commands fail
  with an actionable error.

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
  aigc-cli models --like recraft --type image
  aigc-cli models --like seedream
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
	cmd.Flags().StringVar(&modelLike, "like", "", "Only list models whose id or name contains this text (case-insensitive)")
	cmd.Flags().BoolVar(&modelsDryRun, "dry-run", false, "Print the equivalent curl request without calling the API")
	cmd.Flags().BoolVar(&modelsListProviders, "list-providers", false, "List configured providers without calling any API")
	return cmd
}

func runModels(cmd *cobra.Command, args []string) error {
	if modelsListProviders {
		return runListProviders()
	}

	p := d.ResolveProvider("models")
	if err := options.RequireBaseURL("models", p); err != nil {
		return err
	}
	isOpenRouter := p.ProviderType == provider.OpenRouter
	isAPIMart := p.ProviderType == provider.APIMart

	// The --type flag wins; otherwise a positional value may name a type.
	mediaType := modelType
	if mediaType == "" && len(args) > 0 && knownMediaTypes[args[0]] {
		mediaType = args[0]
	}
	// A --like search without --type spans every modality; the default text
	// listing would never surface image or video models.
	if mediaType == "" && modelLike != "" && isOpenRouter {
		mediaType = "all"
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
			if !isAPIMart {
				return errNoCatalog(p)
			}
			return runModelsPricing(priceModel, p.BaseURL)
		}
		return runModelsCatalog(p, mediaType, isOpenRouter, isAPIMart)
	}

	if modelType != "" {
		return runModelsCatalog(p, mediaType, isOpenRouter, isAPIMart)
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
		return runModelsCatalog(p, mediaType, isOpenRouter, isAPIMart)
	}

	return runModelsOpenAI(p)
}

// runModelsCatalog dispatches a --type listing. Only OpenRouter and APIMart
// expose dedicated catalog endpoints; every other OpenAI-compatible provider
// (ModelScope, relays, …) would otherwise silently query APIMart's marketplace.
func runModelsCatalog(p *provider.EffectiveProvider, mediaType string, isOpenRouter, isAPIMart bool) error {
	switch {
	case isOpenRouter:
		return runModelsOpenRouterDiscovery(p, mediaType)
	case isAPIMart:
		return runModelsMarketplace(mediaType, p.BaseURL)
	default:
		return errNoCatalog(p)
	}
}

// errNoCatalog reports that the resolved provider has no model catalog, instead
// of falling back to an unrelated provider's marketplace API.
func errNoCatalog(p *provider.EffectiveProvider) error {
	target := "this endpoint"
	if p != nil && p.Name != "" {
		target = fmt.Sprintf("provider %q", p.Name)
	}
	typ := provider.Unknown
	if p != nil {
		typ = p.ProviderType
	}
	return fmt.Errorf(
		"%s (%s) does not expose a model catalog: --type and --price require an APIMart or OpenRouter provider; use --provider apimart / --provider openrouter, or omit --type to list its OpenAI-compatible /v1/models",
		target, typ,
	)
}

// matchesLike reports whether any of the given strings contains the --like
// filter (case-insensitive). An empty filter matches everything.
func matchesLike(values ...string) bool {
	if modelLike == "" {
		return true
	}
	needle := strings.ToLower(modelLike)
	for _, v := range values {
		if strings.Contains(strings.ToLower(v), needle) {
			return true
		}
	}
	return false
}

// printNoLikeMatches reports an empty filtered listing.
func printNoLikeMatches() {
	fmt.Printf("No models match --like %q.\n", modelLike)
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
