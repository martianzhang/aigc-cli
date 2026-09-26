package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runModelsOpenRouterDiscovery fetches models from OpenRouter's model discovery endpoints.
// image → GET /api/v1/images/models, video → GET /api/v1/videos/models
func runModelsOpenRouterDiscovery(mediaType string) error {
	base := d.APIBase
	if base == "" {
		return fmt.Errorf("OpenRouter base URL is not configured")
	}
	base = strings.TrimRight(base, "/")

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

// runModelsOpenAI fetches and displays models from OpenAI-compatible /v1/models.
func runModelsOpenAI(p *provider.EffectiveProvider) error {
	if err := options.RequireAPIKey("models", p, d.Providers); err != nil {
		return err
	}
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
		displayID := strings.TrimPrefix(m.ID, "models/")
		line := fmt.Sprintf("  %s", displayID)
		if m.OwnedBy != "" && m.OwnedBy != "openai" && m.OwnedBy != "custom" {
			line += fmt.Sprintf("  (by %s)", m.OwnedBy)
		}
		fmt.Println(line)
	}
	fmt.Println()
	return nil
}

// runModelsDetail fetches and displays a single model by filtering the
// provider's model list. OpenRouter is special-cased to its singular
// /model/{author}/{slug} endpoint, which returns the full model record.
func runModelsDetail(modelID string, p *provider.EffectiveProvider) error {
	if err := options.RequireAPIKey("models", p, d.Providers); err != nil {
		return err
	}
	if p.ProviderType == provider.OpenRouter {
		return runModelsOpenRouterDetail(modelID, p)
	}

	base := p.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	base = strings.TrimRight(base, "/")
	if !client.HasVersionSuffix(base) {
		base += "/v1"
	}
	return runModelsDetailFromList(client.NewFromProvider(p), base, modelID)
}

// runModelsDetailFromList resolves a model by fetching the list and matching the
// id. The list is the one endpoint every OpenAI-compatible provider serves;
// many (ModelScope, Ollama, LM Studio, APIMart, DeepSeek, Groq, …) reject the
// OpenAI-style GET /models/{id} with 404, so it is never probed.
func runModelsDetailFromList(c *client.Client, base, modelID string) error {
	printAPIURL(base + "/models")

	models, err := c.ListModelsOpenAI()
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}
	for i := range models {
		if models[i].ID == modelID {
			printOpenAIModelDetail(&models[i])
			return nil
		}
	}
	return fmt.Errorf("model %q not found", modelID)
}

func printOpenAIModelDetail(model *types.OpenAIModel) {
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
}

// runModelsOpenRouterDetail prints OpenRouter's single-model response verbatim.
// It uses the singular /model/{author}/{slug} endpoint; the plural /models/{id}
// path (used above for OpenAI) returns 404.
func runModelsOpenRouterDetail(modelID string, p *provider.EffectiveProvider) error {
	if p.BaseURL == "" {
		return fmt.Errorf("OpenRouter base URL is not configured")
	}
	base := client.NormalizeBaseURL(p.BaseURL)
	printAPIURL(base + "/model/" + modelID)

	c := client.NewFromProvider(p)
	raw, err := c.GetModelOpenRouter(modelID)
	if err != nil {
		if errors.Is(err, client.ErrNotSupported) {
			return fmt.Errorf("model %q not found", modelID)
		}
		return fmt.Errorf("failed to get model: %w", err)
	}

	pretty, err := indentJSON(raw)
	if err != nil {
		return fmt.Errorf("failed to format response: %w", err)
	}
	fmt.Println(pretty)
	return nil
}

func indentJSON(raw json.RawMessage) (string, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}
