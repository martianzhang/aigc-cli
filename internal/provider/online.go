package provider

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// DescribeImage sends an image to an online LLM for description/vision.
func DescribeImage(p *EffectiveProvider, imagePath, prompt string) (string, error) {
	return visionChat(p, imagePath, prompt)
}

// OCRImage sends an image to an online LLM for OCR text extraction.
// If customPrompt is empty, uses a model-appropriate default.
// Users can override via --prompt flag for custom needs.
func OCRImage(p *EffectiveProvider, imagePath, customPrompt string) (string, error) {
	prompt := customPrompt
	if prompt == "" {
		prompt = "请识别图中的文字"
	}
	return visionChat(p, imagePath, prompt)
}

// visionChat performs a chat completion with an image input.
// For Ollama providers, uses the native /api/chat endpoint with images array.
// For all others, uses the standard OpenAI-compatible /v1/chat/completions.
func visionChat(p *EffectiveProvider, imagePath, textPrompt string) (string, error) {
	encoded, mimeType, err := encodeImage(imagePath)
	if err != nil {
		return "", fmt.Errorf("encode image: %w", err)
	}

	if p.Type == types.ProviderOllama {
		return ollamaNativeChat(p, encoded, textPrompt)
	}
	if p.Type == types.ProviderAnthropic {
		return anthropicVisionChat(p, encoded, mimeType, textPrompt)
	}

	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
	return openaiCompatVisionChat(p, dataURL, textPrompt)
}

// ollamaNativeChat uses Ollama's native /api/chat endpoint with an images array.
// Does NOT send the "options" parameter by default — some custom Ollama
// models (e.g. glm-ocr with custom RENDERER/PARSER) break when options
// is present, while others read it from their Modelfile directly.
//
// Model-specific prompt rules (from upstream docs):
//
//	deepseek-ocr: content = "/path/to/image\\n<prompt>"
//	  Prompts: "Free OCR.", "Extract the text in the image.",
//	           "<|grounding|>Given the layout of the image.",
//	           "<|grounding|>Convert the document to markdown."
//	  We pass image via images[] array, so content is just the prompt.
//	  The <|grounding|> prefix enables layout-aware extraction.
//
//	glm-ocr: content = "<Task>: /path/to/image.png"
//	  Tasks: "Text Recognition:", "Table Recognition:", "Figure Recognition:"
//	  The image path suffix is omitted in API mode (image is in images[]).
func ollamaNativeChat(p *EffectiveProvider, b64Image, textPrompt string) (string, error) {
	model := p.Model
	if model == "" {
		model = "gpt-4o"
	}
	modelKey := ollamaModelKey(model)

	// Build model-specific prompt from the generic textPrompt.
	content := ollamaPrompt(modelKey, textPrompt)

	payload := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": content,
				"images":  []string{b64Image},
			},
		},
		"stream": false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	// Build URL: use base URL without any /v1 suffix, append /api/chat
	baseURL := strings.TrimRight(p.BaseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	apiURL := baseURL + "/api/chat"

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 120 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	return strings.TrimSpace(result.Message.Content), nil
}

// openaiCompatVisionChat uses the standard OpenAI-compatible /v1/chat/completions
// with an image_url in the content array.
func openaiCompatVisionChat(p *EffectiveProvider, dataURL, textPrompt string) (string, error) {
	model := p.Model
	if model == "" {
		model = "gpt-4o"
	}

	payload := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": textPrompt},
					map[string]any{
						"type":      "image_url",
						"image_url": map[string]string{"url": dataURL},
					},
				},
			},
		},
		"max_tokens": 4096,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	baseURL := strings.TrimRight(p.BaseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") && !hasVersionSuffix(baseURL) {
		baseURL += "/v1"
	}
	chatURL := baseURL + "/chat/completions"

	req, err := http.NewRequest("POST", chatURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	httpClient := &http.Client{Timeout: 120 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from model")
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// anthropicVisionChat uses the Anthropic Messages API for image understanding.
// Image is sent as a content block in the messages array.
func anthropicVisionChat(p *EffectiveProvider, b64Image, mimeType, textPrompt string) (string, error) {
	model := p.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	payload := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": textPrompt},
					map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": mimeType,
							"data":       b64Image,
						},
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	baseURL := strings.TrimRight(p.BaseURL, "/")
	chatURL := baseURL + "/messages"

	req, err := http.NewRequest("POST", chatURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	httpClient := &http.Client{Timeout: 120 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	var text string
	for _, block := range result.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return strings.TrimSpace(text), nil
}

// hasVersionSuffix checks if a URL ends with a version path segment like /v1, /v2.
func hasVersionSuffix(urlStr string) bool {
	trimmed := strings.TrimRight(urlStr, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return false
	}
	last := parts[len(parts)-1]
	if len(last) >= 2 && last[0] == 'v' {
		for _, c := range last[1:] {
			if c < '0' || c > '9' {
				return false
			}
		}
		return len(last) > 1
	}
	return false
}

// encodeImage reads an image file and returns base64-encoded data with MIME type.
func encodeImage(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	ext := strings.ToLower(filepath.Ext(path))
	return base64.StdEncoding.EncodeToString(data), mimeFromExt(ext), nil
}

func mimeFromExt(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"
	default:
		return "image/png"
	}
}

// ollamaModelKey extracts a simplified model key from a model name.
// "deepseek-ocr:latest" → "deepseek-ocr", "glm-ocr" → "glm-ocr"
func ollamaModelKey(model string) string {
	model = strings.Split(model, ":")[0]
	model = strings.Split(model, "/")[0] // handle namespace like "x/model"
	return model
}

// ollamaPrompt builds a model-specific prompt from a generic OCR/vision request.
// Different Ollama models have different prompt format expectations.
func ollamaPrompt(modelKey, genericPrompt string) string {
	switch modelKey {
	case "deepseek-ocr":
		// deepseek-ocr uses the content field as a direct instruction.
		// The <|grounding|> prefix enables layout-aware extraction but
		// returns structured HTML which isn't useful for plain OCR.
		// Use simple, direct prompts for best plain-text results.
		if strings.Contains(genericPrompt, "识别图中的文字") || strings.Contains(genericPrompt, "extract") {
			return "Extract the text in the image."
		}
		return genericPrompt

	case "glm-ocr":
		// glm-ocr expects "<Task>:" prefix. The image path suffix is
		// omitted because we pass the image via the images[] array.
		if strings.Contains(genericPrompt, "识别图中的文字") || strings.Contains(genericPrompt, "extract") {
			return "Text Recognition:"
		}
		// Default to Text Recognition for unknown tasks.
		return "Text Recognition:"

	default:
		// Generic multimodal model (llava, gemma3 vision, etc).
		return genericPrompt
	}
}
