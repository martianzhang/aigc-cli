package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

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
