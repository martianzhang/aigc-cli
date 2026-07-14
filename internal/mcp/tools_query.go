package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/provider"
	"github.com/martianzhang/apimart-cli/internal/service"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// listModelsHandler creates the handler for list_models.
func listModelsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		mediaType := request.GetString("type", "")

		models, err := fetchModels("", mediaType, "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch models: %v", err)), nil
		}

		if len(models) == 0 {
			return mcp.NewToolResultText("没有找到模型。"), nil
		}

		vendorMap := make(map[string][]types.MarketplaceModel)
		for _, m := range models {
			vName := "Other"
			if m.Vendor != nil && m.Vendor.Name != "" {
				vName = m.Vendor.Name
			}
			vendorMap[vName] = append(vendorMap[vName], m)
		}

		var b strings.Builder
		title := "模型列表"
		if mediaType != "" {
			title = strings.ToUpper(mediaType[:1]) + mediaType[1:] + " 模型"
		}
		fmt.Fprintf(&b, "%s (%d total)\n\n", title, len(models))

		for vName, mods := range vendorMap {
			fmt.Fprintf(&b, "%s:\n", vName)
			for _, m := range mods {
				price := m.FormatPrice()
				fmt.Fprintf(&b, "  %-30s  %s\n", m.ModelName, price)
			}
			b.WriteString("\n")
		}

		return mcp.NewToolResultText(b.String()), nil
	}
}

// getModelPricingHandler creates the handler for get_model_pricing.
func getModelPricingHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		modelName, err := request.RequireString("model")
		if err != nil {
			return mcp.NewToolResultError("model is required"), nil
		}

		// Use main domain for pricing API
		baseURL := "https://apimart.ai"
		url := fmt.Sprintf("%s/api/pricing/model?model=%s", baseURL, modelName)

		var result types.ModelPricingResponse
		// Use simple HTTP call since pricing doesn't need auth
		resp, err := http.DefaultClient.Get(url)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to fetch pricing: %v", err)), nil
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to read response: %v", err)), nil
		}
		if resp.StatusCode != http.StatusOK {
			return mcp.NewToolResultError(fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(body))), nil
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse response: %v", err)), nil
		}
		if !result.Success {
			return mcp.NewToolResultError("API returned error"), nil
		}

		d := result.Data
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n", d.ModelName)
		fmt.Fprintf(&b, "  Billing: %s\n", d.BillingType)
		fmt.Fprintf(&b, "  Base price: $%.5f\n", d.ModelPrice)
		fmt.Fprintf(&b, "  Qualities: %s\n", strings.Join(d.SupportedQualities, ", "))

		if d.BillingType == "size_quality" && len(d.SizeQualityPrices) > 0 {
			b.WriteString("\n  Size x Quality pricing:\n")
			for size, qMap := range d.SizeQualityPrices {
				for q, p := range qMap {
					fmt.Fprintf(&b, "    %-14s  %-6s  $%.5f\n", size, q, p)
				}
			}
		}

		return mcp.NewToolResultText(b.String()), nil
	}
}

// getBalanceHandler creates the handler for get_balance.
// Calls both token balance and user balance, returns combined result.
func getBalanceHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if cfg.APIKey == "" {
			return mcp.NewToolResultError("API Key not configured"), nil
		}

		c := client.New(cfg.APIKey, cfg.BaseURL, cfg.Proxy)

		var b strings.Builder
		b.WriteString("=== Token Balance ===\n")

		tokenBal, err := c.GetTokenBalance()
		if err != nil {
			fmt.Fprintf(&b, "查询失败: %v\n", err)
		} else if tokenBal.Success {
			if tokenBal.UnlimitedQuota {
				b.WriteString("  Status: Unlimited Quota (no limit)\n")
			} else {
				fmt.Fprintf(&b, "  Remain Balance: $%.4f\n", tokenBal.RemainBalance)
				fmt.Fprintf(&b, "  Remain Credits: %.4f\n", tokenBal.RemainCredits)
			}
			fmt.Fprintf(&b, "  Used Balance: $%.4f\n", tokenBal.UsedBalance)
			fmt.Fprintf(&b, "  Used Credits: %.4f\n", tokenBal.UsedCredits)
		} else {
			fmt.Fprintf(&b, "  Error: %s\n", tokenBal.Message)
		}

		b.WriteString("\n=== User Balance ===\n")

		userBal, err := c.GetUserBalance()
		if err != nil {
			fmt.Fprintf(&b, "查询失败: %v\n", err)
		} else if userBal.Success {
			fmt.Fprintf(&b, "  Remain Balance: $%.4f\n", userBal.RemainBalance)
			fmt.Fprintf(&b, "  Remain Credits: %.4f\n", userBal.RemainCredits)
			fmt.Fprintf(&b, "  Used Balance: $%.4f\n", userBal.UsedBalance)
			fmt.Fprintf(&b, "  Used Credits: %.4f\n", userBal.UsedCredits)
		} else {
			fmt.Fprintf(&b, "  Error: %s\n", userBal.Message)
		}

		return mcp.NewToolResultText(b.String()), nil
	}
}

// getTaskHandler creates the handler for get_task.
// Supports APIMart task_id and OpenRouter job_id (video).
func getTaskHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if cfg.APIKey == "" {
			return mcp.NewToolResultError("API Key not configured"), nil
		}

		taskID, err := request.RequireString("task_id")
		if err != nil {
			return mcp.NewToolResultError("task_id is required"), nil
		}

		c := client.New(cfg.APIKey, cfg.BaseURL, cfg.Proxy)
		p := provider.Detect(cfg.BaseURL)

		if p == provider.OpenRouter {
			return handleMCPGetOpenRouterJob(c, taskID, cfg.Output, cfg.APIKey)
		}
		return handleMCPGetAPIMartTask(c, taskID, cfg.Output)
	}
}

func handleMCPGetOpenRouterJob(c client.APIClient, jobID, outputDir, apiKey string) (*mcp.CallToolResult, error) {
	statusResp, err := c.OpenRouterVideoGet(jobID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query job: %v", err)), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Job ID: %s\n", statusResp.ID)
	fmt.Fprintf(&b, "Status: %s\n", statusResp.Status)

	switch statusResp.Status {
	case "completed":
		b.WriteString("\n视频:\n")
		for i, u := range statusResp.UnsignedURLs {
			fullpath, err := service.DownloadFile(u, outputDir, fmt.Sprintf("video_%s_%d", jobID, i))
			if err == nil {
				fmt.Fprintf(&b, "  %s\n", fullpath)
			} else {
				// Unsigned URL failed; try authenticated fallback
				fallback := fmt.Sprintf("https://openrouter.ai/api/v1/videos/%s/content?index=%d", jobID, i)
				fullpath, err = service.DownloadFile(fallback, outputDir, fmt.Sprintf("video_%s_%d", jobID, i))
				if err == nil {
					fmt.Fprintf(&b, "  %s\n", fullpath)
				} else {
					fmt.Fprintf(&b, "  %s (download failed: %v)\n", u, err)
				}
			}
		}
		if statusResp.Usage != nil && statusResp.Usage.TotalCost > 0 {
			fmt.Fprintf(&b, "\nCost: $%.5f\n", statusResp.Usage.TotalCost)
		}
	case "failed", "cancelled", "expired":
		fmt.Fprintf(&b, "Error: %s\n", statusResp.Error)
	default:
		b.WriteString("\n任务仍在处理中，请稍后再查。\n")
	}

	return mcp.NewToolResultText(b.String()), nil
}

func handleMCPGetAPIMartTask(c client.APIClient, taskID, outputDir string) (*mcp.CallToolResult, error) {
	task, err := c.GetTask(taskID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query task: %v", err)), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Task ID: %s\n", task.ID)
	fmt.Fprintf(&b, "Status: %s | Progress: %d%%\n", task.Status, task.Progress)

	switch task.Status {
	case "completed":
		fmt.Fprintf(&b, "Time: %ds | Cost: $%.5f (%.4f credits)\n", task.ActualTime, task.Cost, task.CreditsCost)

		if task.Result != nil && len(task.Result.Images) > 0 {
			b.WriteString("\n图片:\n")
			for i, img := range task.Result.Images {
				for j, url := range img.URL {
					fullpath, err := service.DownloadFile(url, outputDir, fmt.Sprintf("image_%s_%d_%d", task.ID, i, j))
					if err == nil {
						fmt.Fprintf(&b, "  %s\n", fullpath)
					} else {
						fmt.Fprintf(&b, "  %s (download failed: %v)\n", url, err)
					}
				}
			}
		}
		if task.Result != nil && len(task.Result.Videos) > 0 {
			b.WriteString("\n视频:\n")
			for i, vid := range task.Result.Videos {
				for j, url := range vid.URL {
					fullpath, err := service.DownloadFile(url, outputDir, fmt.Sprintf("video_%s_%d_%d", task.ID, i, j))
					if err == nil {
						fmt.Fprintf(&b, "  %s\n", fullpath)
					} else {
						fmt.Fprintf(&b, "  %s (download failed: %v)\n", url, err)
					}
				}
			}
		}
	case "failed":
		if task.Error != nil {
			fmt.Fprintf(&b, "Error: %s (code %d)\n", task.Error.Message, task.Error.Code)
		}
	default:
		b.WriteString("\n任务仍在处理中，请稍后再查。\n")
	}

	return mcp.NewToolResultText(b.String()), nil
}

// fetchModels gets models from the marketplace API.
func fetchModels(baseURL, mediaType, _ string) ([]types.MarketplaceModel, error) {
	if baseURL == "" {
		baseURL = "https://api.apimart.ai"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1") // marketplace API doesn't use /v1 prefix

	// Reuse the global DefaultClient which is configured with proxy at startup
	c := http.DefaultClient
	pageSize := 50
	page := 1
	var allModels []types.MarketplaceModel
	var total int

	for {
		u := fmt.Sprintf("%s/api/marketplace/models?sort=newest&page=%d&page_size=%d", baseURL, page, pageSize)
		if mediaType != "" {
			u += "&type=" + mediaType
		}

		resp, err := c.Get(u)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch models (page %d): %w", page, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		var result types.MarketplaceResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		if !result.Success {
			return nil, fmt.Errorf("API returned error")
		}

		total = result.Data.Total
		allModels = append(allModels, result.Data.Models...)

		if len(allModels) >= total {
			break
		}
		page++
	}

	return allModels, nil
}
