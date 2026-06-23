package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/types"
)

var modelType string

// modelsCmd represents the `apimart-cli models` command.
var modelsCmd = &cobra.Command{
	Use:   "models [image|video|chat]",
	Short: "List available AI models from APIMart marketplace",
	Long: `Query and display models from the APIMart marketplace.

Supports filtering by type: image, video, or chat. No API key required.

Examples:
  apimart-cli models
  apimart-cli models image
  apimart-cli models video
  apimart-cli models chat`,
	Args: cobra.MaximumNArgs(1),
	RunE: runModels,
}

func runModels(cmd *cobra.Command, args []string) error {
	// Determine type filter
	mediaType := ""
	if len(args) > 0 {
		mediaType = args[0]
	}
	if modelType != "" {
		mediaType = modelType
	}

	// Check if we should use APIMart marketplace or OpenAI-compatible /v1/models
	if !isAPIMartProvider() {
		return runModelsOpenAI()
	}

	base := apiBase
	if base == "" {
		base = "https://api.apimart.ai"
	}
	base = strings.TrimRight(base, "/")

	httpClient := httpProxyClient()
	pageSize := 50
	page := 1
	var allModels []types.MarketplaceModel
	var total int

	for {
		url := fmt.Sprintf("%s/api/marketplace/models?sort=newest&page=%d&page_size=%d", base, page, pageSize)
		if mediaType != "" {
			url += "&type=" + mediaType
		}

		resp, err := httpClient.Get(url)
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
	fmt.Printf("\n%s (%d total)\n\n", title, total)

	for _, g := range groups {
		fmt.Printf("  %s:\n", g.vendor)
		for _, m := range g.models {
			price := formatPrice(m)
			tags := strings.Join(m.Tags, ", ")
			line := fmt.Sprintf("    %-30s", m.ModelName)
			line += fmt.Sprintf("  %-12s", price)
			if tags != "" {
				line += "  " + tags
			}
			fmt.Println(line)
		}
		fmt.Println()
	}
	return nil
}

// runModelsOpenAI fetches and displays models from OpenAI-compatible /v1/models.
func runModelsOpenAI() error {
	c := client.New(apiKey, apiBase, httpProxy)
	models, err := c.ListModelsOpenAI()
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	if len(models) == 0 {
		fmt.Println("No models found.")
		return nil
	}

	fmt.Printf("\nAvailable models (%d):\n\n", len(models))
	for _, m := range models {
		line := fmt.Sprintf("  %s", m.ID)
		if m.OwnedBy != "" && m.OwnedBy != "openai" {
			line += fmt.Sprintf("  (by %s)", m.OwnedBy)
		}
		fmt.Println(line)
	}
	fmt.Println()
	return nil
}

func formatPrice(m types.MarketplaceModel) string {
	if !m.Pricing.HasPrice {
		return "—"
	}
	unit := m.Pricing.PriceUnit
	if unit == "" {
		unit = "/次"
	}
	switch m.Pricing.BillingType {
	case "per_token":
		return fmt.Sprintf("$%.4f%s", m.Pricing.StartingPrice, unit)
	default:
		return fmt.Sprintf("$%.4f%s", m.Pricing.StartingPrice, unit)
	}
}

// pricingCmd represents `apimart-cli models pricing <name>`.
var pricingCmd = &cobra.Command{
	Use:   "pricing <model-name>",
	Short: "Show detailed pricing for a specific model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		modelName := args[0]
		base := mainDomain(apiBase)
		url := base + "/api/pricing/model?model=" + modelName

		client := httpProxyClient()
		resp, err := client.Get(url)
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

		fmt.Printf("\n%s\n", d.ModelName)
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
	},
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

// httpProxyClient returns an HTTP client that respects the configured proxy.
func httpProxyClient() *http.Client {
	transport := &http.Transport{}
	proxyURL := httpProxy
	if proxyURL == "" {
		proxyURL = os.Getenv("APIMART_HTTP_PROXY")
	}
	if proxyURL == "" {
		proxyURL = os.Getenv("HTTP_PROXY")
	}
	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(parsed)
		}
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{Transport: transport}
}

func init() {
	modelsCmd.Flags().StringVarP(&modelType, "type", "t", "", "Filter by type: image, video, chat")
	modelsCmd.AddCommand(pricingCmd)
	rootCmd.AddCommand(modelsCmd)
}
