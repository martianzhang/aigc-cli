package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// openRouterModalities maps user-facing --type values (including aliases) to
// OpenRouter's output_modalities filter values.
var openRouterModalities = map[string]string{
	"text":          "text",
	"chat":          "text",
	"llm":           "text",
	"image":         "image",
	"video":         "video",
	"audio":         "audio",
	"music":         "audio",
	"speech":        "speech",
	"tts":           "speech",
	"transcription": "transcription",
	"stt":           "transcription",
	"asr":           "transcription",
	"embeddings":    "embeddings",
	"embedding":     "embeddings",
	"embed":         "embeddings",
	"rerank":        "rerank",
	"reranking":     "rerank",
	"decisions":     "decisions",
	"decision":      "decisions",
	"all":           "all",
}

// openRouterModalityOrder is the canonical order used in help text and errors.
var openRouterModalityOrder = []string{
	"text", "image", "video", "audio", "speech",
	"transcription", "embeddings", "rerank", "decisions", "all",
}

// openRouterModalityTitles labels each output modality in listings.
var openRouterModalityTitles = map[string]string{
	"text":          "Text / Chat Models",
	"image":         "Image Models",
	"video":         "Video Models",
	"audio":         "Audio Models",
	"speech":        "Text-to-Speech Models",
	"transcription": "Speech-to-Text Models",
	"embeddings":    "Embedding Models",
	"rerank":        "Rerank Models",
	"decisions":     "Decision Models",
	"all":           "All Models",
}

// normalizeOpenRouterModality validates and canonicalizes a --type value.
// An empty value means the default text/chat listing.
func normalizeOpenRouterModality(mediaType string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(mediaType))
	if m == "" {
		return "text", nil
	}
	if canonical, ok := openRouterModalities[m]; ok {
		return canonical, nil
	}
	return "", fmt.Errorf("unknown model type %q for OpenRouter; supported: %s",
		mediaType, strings.Join(openRouterModalityOrder, ", "))
}

// runModelsOpenRouterDiscovery fetches models from OpenRouter's discovery
// endpoints. Every output modality is supported:
//
//	image, video → GET /api/v1/images/models, /api/v1/videos/models
//	               (generation metadata: resolutions, aspect ratios, params)
//	everything   → GET /api/v1/models?output_modalities=<modality>
//	               (standard Models API: text, audio, speech, transcription,
//	                embeddings, rerank, decisions, all)
//
// Image/video fall back to the standard endpoint when --price is requested,
// because only the standard response carries pricing.
func runModelsOpenRouterDiscovery(p *provider.EffectiveProvider, mediaType string) error {
	if p.BaseURL == "" {
		return fmt.Errorf("OpenRouter base URL is not configured")
	}
	modality, err := normalizeOpenRouterModality(mediaType)
	if err != nil {
		return err
	}
	base := client.NormalizeBaseURL(p.BaseURL)

	if !cmdPriceChanged && (modality == "image" || modality == "video") {
		return runOpenRouterMediaDiscovery(base, modality)
	}
	return runOpenRouterStandardDiscovery(base, modality)
}

// runOpenRouterMediaDiscovery lists image/video models from the dedicated
// generation endpoints, which expose parameter ranges and resolutions.
func runOpenRouterMediaDiscovery(base, modality string) error {
	endpoint := base + "/" + modality + "s/models"
	printAPIURL(endpoint)
	if modelsDryRun {
		printModelsCurl(endpoint, "", "")
		return nil
	}

	body, err := fetchOpenRouterModels(endpoint)
	if err != nil {
		return err
	}

	var list types.OpenRouterMediaModelList
	if err := json.Unmarshal(body, &list); err != nil {
		return fmt.Errorf("failed to parse response from %s: %w", endpoint, err)
	}

	if len(list.Data) == 0 {
		fmt.Println("No models found.")
		return nil
	}

	fmt.Printf("%s (%d)\n\n", openRouterModalityTitles[modality], len(list.Data))
	for _, m := range list.Data {
		printOpenRouterMediaModel(m)
	}
	return nil
}

func printOpenRouterMediaModel(m types.OpenRouterMediaModel) {
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
				v = append(append([]string{}, v[:6]...), "...")
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

// runOpenRouterStandardDiscovery lists models through the standard Models API
// filtered by output modality.
func runOpenRouterStandardDiscovery(base, modality string) error {
	endpoint := base + "/models?output_modalities=" + url.QueryEscape(modality)
	printAPIURL(endpoint)
	if modelsDryRun {
		printModelsCurl(endpoint, "", "")
		return nil
	}

	body, err := fetchOpenRouterModels(endpoint)
	if err != nil {
		return err
	}

	var list types.OpenRouterModelList
	if err := json.Unmarshal(body, &list); err != nil {
		return fmt.Errorf("failed to parse response from %s: %w", endpoint, err)
	}

	if len(list.Data) == 0 {
		fmt.Println("No models found.")
		return nil
	}

	fmt.Printf("%s (%d)\n\n", openRouterModalityTitles[modality], len(list.Data))
	// Text and all-modality listings are long (hundreds of models), so they
	// stay one line per model. Every other modality shows capabilities.
	compact := modality == "text" || modality == "all"
	for _, m := range list.Data {
		printOpenRouterModel(m, compact, modality == "all", cmdPriceChanged)
	}
	return nil
}

func printOpenRouterModel(m types.OpenRouterModel, compact, showModality, showPrice bool) {
	line := "  " + m.ID
	if showModality && m.Architecture != nil && len(m.Architecture.OutputModalities) > 0 {
		line += "  [" + strings.Join(m.Architecture.OutputModalities, ", ") + "]"
	}
	if m.IsFree() {
		line += "  (free)"
	}
	if compact && showPrice {
		line += "  " + formatOpenRouterPrice(m.Pricing)
	}
	fmt.Println(line)
	if compact {
		return
	}

	if m.Name != "" && m.Name != m.ID {
		fmt.Printf("    ─ %s\n", m.Name)
	}
	if m.Architecture != nil {
		fmt.Printf("    ─ %s → %s\n",
			joinOrDash(m.Architecture.InputModalities),
			joinOrDash(m.Architecture.OutputModalities))
	}
	if len(m.SupportedVoices) > 0 {
		fmt.Printf("    ─ voices: %s\n", formatVoiceList(m.SupportedVoices))
	}
	if showPrice {
		fmt.Printf("    ─ %s\n", formatOpenRouterPrice(m.Pricing))
	}
	if m.ExpirationDate != nil && *m.ExpirationDate != "" {
		fmt.Printf("    ─ expires %s\n", *m.ExpirationDate)
	}
	fmt.Println()
}

// joinOrDash joins modality names, falling back to "-" when unknown.
func joinOrDash(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	return strings.Join(items, ", ")
}

// maxVoicesShown caps the voice list in listings; some TTS models expose
// dozens of voices.
const maxVoicesShown = 12

// formatVoiceList renders a voice list, truncating long ones with a count.
func formatVoiceList(voices []string) string {
	if len(voices) <= maxVoicesShown {
		return strings.Join(voices, ", ")
	}
	return fmt.Sprintf("%s, … (%d total)", strings.Join(voices[:maxVoicesShown], ", "), len(voices))
}

// formatOpenRouterPrice renders OpenRouter's per-unit prices compactly.
// Prompt/completion prices are per token, so they are shown per million.
func formatOpenRouterPrice(p types.OpenRouterPricing) string {
	if p.IsFree() {
		return "free"
	}
	fields := []struct {
		key   string
		label string
		perM  bool
	}{
		{"prompt", "in", true},
		{"completion", "out", true},
		{"image", "image", false},
		{"image_output", "image out", true},
		{"audio", "audio", false},
		{"audio_output", "audio out", true},
		{"request", "request", false},
	}
	var parts []string
	for _, f := range fields {
		v, ok := p.Price(f.key)
		if !ok || v <= 0 {
			continue
		}
		if f.perM {
			parts = append(parts, "$"+trimPrice(v*1e6)+"/M "+f.label)
		} else {
			parts = append(parts, "$"+trimPrice(v)+"/"+f.label)
		}
	}
	if len(parts) == 0 {
		return "n/a"
	}
	return strings.Join(parts, " · ")
}

// trimPrice formats a price with at most 4 significant digits.
func trimPrice(v float64) string {
	return strconv.FormatFloat(v, 'g', 4, 64)
}

// fetchOpenRouterModels performs the GET and normalizes transport/HTTP errors.
func fetchOpenRouterModels(endpoint string) ([]byte, error) {
	resp, err := http.DefaultClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}
	if isHTML(body) {
		return nil, fmt.Errorf("expected JSON response but got HTML — check proxy or network connectivity to %s", endpoint)
	}
	return body, nil
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
	if modelsDryRun {
		printModelsCurl(base+"/models", p.APIKey, "")
		return nil
	}

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
	if p.ProviderType == provider.OpenRouter {
		fmt.Println()
		fmt.Println("Other modalities: aigc-cli models --type all")
		fmt.Println("Types: text, image, video, audio, speech, transcription, embeddings, rerank, decisions, all")
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
	printAPIURL(base + "/models")
	if modelsDryRun {
		printModelsCurl(base+"/models", p.APIKey, "")
		return nil
	}
	return runModelsDetailFromList(client.NewFromProvider(p), modelID)
}

// runModelsDetailFromList resolves a model by fetching the list and matching the
// id. The list is the one endpoint every OpenAI-compatible provider serves;
// many (ModelScope, Ollama, LM Studio, APIMart, DeepSeek, Groq, …) reject the
// OpenAI-style GET /models/{id} with 404, so it is never probed.
func runModelsDetailFromList(c *client.Client, modelID string) error {
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
	if modelsDryRun {
		printModelsCurl(base+"/model/"+modelID, p.APIKey, "")
		return nil
	}

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
