package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
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

// runModelsMarketplace fetches models from the APIMart marketplace API.
// The marketplace is a public API (no auth required).
func runModelsMarketplace(mediaType string) error {
	base := shared.APIBase
	if base == "" {
		base = "https://api.apimart.ai"
	}
	base = strings.TrimRight(base, "/")
	base = strings.TrimSuffix(base, "/v1") // marketplace API doesn't use /v1 prefix

	firstURL := fmt.Sprintf("%s/api/marketplace/models?sort=newest&page=1&page_size=50", base)
	if mediaType != "" {
		firstURL += "&type=" + mediaType
	}
	printAPIURL(firstURL)

	pageSize := 50
	page := 1
	var allModels []types.MarketplaceModel
	var total int

	for {
		url := fmt.Sprintf("%s/api/marketplace/models?sort=newest&page=%d&page_size=%d", base, page, pageSize)
		if mediaType != "" {
			url += "&type=" + mediaType
		}

		resp, err := http.DefaultClient.Get(url)
		if err != nil {
			return fmt.Errorf("failed to fetch models (page %d): %w", page, err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		var result types.MarketplaceResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if !result.Success {
			return fmt.Errorf("API returned error")
		}

		total = result.Data.Total
		allModels = append(allModels, result.Data.Models...)

		if len(allModels) >= total {
			break
		}
		page++
	}

	if len(allModels) == 0 {
		fmt.Println("No models found.")
		return nil
	}

	// Build vendor → models map
	type group struct {
		vendor string
		models []types.MarketplaceModel
	}
	var groups []group
	vendorMap := make(map[string][]types.MarketplaceModel)

	for _, m := range allModels {
		vName := "Other"
		if m.Vendor != nil && m.Vendor.Name != "" {
			vName = m.Vendor.Name
		}
		vendorMap[vName] = append(vendorMap[vName], m)
	}

	for vName, mods := range vendorMap {
		groups = append(groups, group{vName, mods})
	}

	// Print header
	title := "All Models"
	if mediaType != "" {
		title = strings.ToUpper(mediaType[:1]) + mediaType[1:] + " Models"
	}
	fmt.Printf("%s (%d total)\n\n", title, total)

	for _, g := range groups {
		fmt.Printf("  %s:\n", g.vendor)
		for _, m := range g.models {
			line := fmt.Sprintf("    %-30s", m.ModelName)
			// --price (bare) adds pricing column
			if cmdPriceChanged {
				line += fmt.Sprintf("  %-12s", m.FormatPrice())
			}
			tags := strings.Join(m.Tags, ", ")
			if tags != "" {
				line += "  " + tags
			}
			fmt.Println(line)
		}
		fmt.Println()
	}
	return nil
}

// runModelsOpenRouterDiscovery fetches models from OpenRouter's model discovery endpoints.
// image → GET /api/v1/images/models, video → GET /api/v1/videos/models
func runModelsOpenRouterDiscovery(mediaType string) error {
	base := shared.APIBase
	if base == "" {
		return fmt.Errorf("OpenRouter base URL is not configured")
	}
	base = strings.TrimRight(base, "/")

	// e.g. https://openrouter.ai/api/v1 + /images/models → https://openrouter.ai/api/v1/images/models
	endpoint := base + "/" + mediaType + "s/models"
	if mediaType == "chat" {
		endpoint = base + "/models"
	}

	printAPIURL(endpoint)

	resp, err := http.DefaultClient.Get(endpoint)
	if err != nil {
		return fmt.Errorf("failed to fetch %s models: %w", mediaType, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Check for HTML response (proxy/gateway returning block page)
	if isHTML(body) {
		return fmt.Errorf("expected JSON response but got HTML — check proxy or network connectivity to %s", endpoint)
	}

	var list types.OpenRouterMediaModelList
	if err := json.Unmarshal(body, &list); err != nil {
		return fmt.Errorf("failed to parse response from %s: %w", endpoint, err)
	}

	if len(list.Data) == 0 {
		fmt.Println("No models found.")
		return nil
	}

	title := strings.ToUpper(mediaType[:1]) + mediaType[1:] + " Models"
	fmt.Printf("%s (%d)\n\n", title, len(list.Data))

	for _, m := range list.Data {
		fmt.Printf("  %s\n", m.ID)
		if m.Name != "" && m.Name != m.ID {
			fmt.Printf("    ─ %s\n", m.Name)
		}
		if m.Architecture != nil {
			in := strings.Join(m.Architecture.InputModalities, ", ")
			out := strings.Join(m.Architecture.OutputModalities, ", ")
			fmt.Printf("    ─ %s → %s\n", in, out)
		}
		if m.SupportsStreaming {
			fmt.Printf("    ─ streaming\n")
		}
		// Show key supported parameters on one line
		var params []string
		for k, desc := range m.SupportedParameters {
			switch desc.Type {
			case "boolean":
				params = append(params, k)
			case "enum":
				v := desc.Values
				if len(v) > 6 {
					v = append(v[:6], "...")
				}
				params = append(params, fmt.Sprintf("%s=%s", k, strings.Join(v, "|")))
			case "range":
				if desc.Min != nil && desc.Max != nil {
					params = append(params, fmt.Sprintf("%s=%d-%d", k, *desc.Min, *desc.Max))
				} else {
					params = append(params, k)
				}
			}
		}
		if len(params) > 0 {
			fmt.Printf("    ─ %s\n", strings.Join(params, ", "))
		}
		fmt.Println()
	}
	return nil
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

// runModelsOpenAI fetches and displays models from OpenAI-compatible /v1/models.
func runModelsOpenAI(p *provider.EffectiveProvider) error {
	base := p.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	base = strings.TrimRight(base, "/")
	if !client.HasVersionSuffix(base) {
		base += "/v1"
	}
	printAPIURL(base + "/models")

	c := client.NewFromProvider(p)
	models, err := c.ListModelsOpenAI()
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	if len(models) == 0 {
		fmt.Println("No models found.")
		return nil
	}

	fmt.Printf("Available models (%d):\n\n", len(models))
	for _, m := range models {
		line := fmt.Sprintf("  %s", m.ID)
		// Only annotate when owned_by provides useful information
		if m.OwnedBy != "" && m.OwnedBy != "openai" && m.OwnedBy != "custom" {
			line += fmt.Sprintf("  (by %s)", m.OwnedBy)
		}
		fmt.Println(line)
	}
	fmt.Println()
	return nil
}

// runModelsDetail fetches and displays a single model via /v1/models/{model}.
func runModelsDetail(modelID string, p *provider.EffectiveProvider) error {
	base := p.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	base = strings.TrimRight(base, "/")
	if !client.HasVersionSuffix(base) {
		base += "/v1"
	}
	printAPIURL(base + "/models/" + modelID)

	c := client.NewFromProvider(p)
	model, err := c.GetModelOpenAI(modelID)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	// Some APIs return HTTP 200 with empty data for non-existent models.
	if model.ID == "" {
		return fmt.Errorf("model %q not found", modelID)
	}

	fmt.Printf("  %s\n", model.ID)
	if model.Object != "" {
		fmt.Printf("    Object:   %s\n", model.Object)
	}
	if model.OwnedBy != "" {
		fmt.Printf("    Owned by: %s\n", model.OwnedBy)
	}
	if model.Created > 0 {
		fmt.Printf("    Created:  %s\n", time.Unix(model.Created, 0).Format("2006-01-02 15:04:05"))
	}
	fmt.Println()
	return nil
}

// runModelsPricing fetches and displays detailed pricing for a single model.
func runModelsPricing(modelName string) error {
	base := mainDomain(shared.APIBase)
	pricingURL := base + "/api/pricing/model?model=" + modelName

	printAPIURL(pricingURL)

	resp, err := http.DefaultClient.Get(pricingURL)
	if err != nil {
		return fmt.Errorf("failed to fetch pricing: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result types.ModelPricingResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("API returned error")
	}
	d := result.Data

	// Validate that the model actually exists on the platform.
	// A non-existent model returns HTTP 200 with success=true but zero-valued data.
	if d.BillingType == "" && d.ModelPrice == 0 {
		return fmt.Errorf("model %q not found", modelName)
	}

	fmt.Printf("%s\n", d.ModelName)
	fmt.Printf("  Billing: %s\n", d.BillingType)
	fmt.Printf("  Base price: $%.5f\n", d.ModelPrice)
	fmt.Printf("  Discount: %.0f%%\n", (1-d.DiscountRate)*100)
	fmt.Printf("  Qualities: %s\n", strings.Join(d.SupportedQualities, ", "))

	if d.BillingType == "size_quality" && len(d.SizeQualityPrices) > 0 {
		fmt.Printf("\n  Size × Quality pricing (lowest):\n")
		// Collect lowest price per size
		type sq struct {
			size, quality string
			price         float64
		}
		var cheapest []sq
		for size, qMap := range d.SizeQualityPrices {
			lowest := sq{size: size, price: 1e9}
			for q, p := range qMap {
				if p < lowest.price {
					lowest.price = p
					lowest.quality = q
				}
			}
			cheapest = append(cheapest, lowest)
		}

		// Sort by price ascending
		for i := 0; i < len(cheapest); i++ {
			for j := i + 1; j < len(cheapest); j++ {
				if cheapest[j].price < cheapest[i].price {
					cheapest[i], cheapest[j] = cheapest[j], cheapest[i]
				}
			}
		}

		for _, s := range cheapest {
			fmt.Printf("    %-14s  %-6s  $%.5f\n", s.size, s.quality, s.price)
		}
	}
	return nil
}

// mainDomain extracts the main domain from an API base URL.
// e.g. "https://api.apimart.ai" → "https://apimart.ai"
func mainDomain(baseURL string) string {
	if baseURL == "" {
		baseURL = "https://api.apimart.ai"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")

	// Replace api. prefix with empty
	if strings.HasPrefix(baseURL, "https://api.") {
		return "https://" + strings.TrimPrefix(baseURL, "https://api.")
	}
	if strings.HasPrefix(baseURL, "http://api.") {
		return "http://" + strings.TrimPrefix(baseURL, "http://api.")
	}
	return baseURL
}

func init() {
	modelsCmd.Flags().StringVarP(&modelType, "type", "t", "", "Filter by media type (APIMart marketplace): image, video, chat")
	modelsCmd.Flags().StringVarP(&priceArg, "price", "p", "", "Show pricing column (no arg) or model pricing details (with model name) (APIMart only)")
	rootCmd.AddCommand(modelsCmd)
}

// printAPIURL prints the API endpoint being called in a consistent format.
func printAPIURL(apiURL string) {
	fmt.Printf("API: %s\n", apiURL)
}
