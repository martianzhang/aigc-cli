package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ollamaGenerateResponse is the response from Ollama's /api/generate for image models.
// Note: some models return "images" (array), others return "image" (single string).
type ollamaGenerateResponse struct {
	Model         string   `json:"model"`
	CreatedAt     string   `json:"created_at"`
	Response      string   `json:"response"`
	Done          bool     `json:"done"`
	DoneReason    string   `json:"done_reason"`
	Images        []string `json:"images,omitempty"`
	Image         string   `json:"image,omitempty"`
	TotalDuration int64    `json:"total_duration,omitempty"`
}

// ollamaGenerateImages sends a request to Ollama's /api/generate and returns saved filenames.
func ollamaGenerateImages(baseURL string, req *types.GenerateRequest) ([]string, error) {
	// Strip version suffix (e.g., /v1) to get raw Ollama endpoint.
	if idx := strings.LastIndex(baseURL, "/v"); idx > strings.LastIndex(baseURL, "://") {
		baseURL = baseURL[:idx]
	}
	url := baseURL + "/api/generate"
	body := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
		"stream": false,
	}

	bodyBytes, _ := json.Marshal(body)
	httpResp, err := http.Post(url, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ollama returned HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	// Collect images: some models return "images" (array), others return "image" (single).
	var images []string
	if len(ollamaResp.Images) > 0 {
		images = ollamaResp.Images
	} else if ollamaResp.Image != "" {
		images = append(images, ollamaResp.Image)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("ollama returned no images")
	}

	var saved []string
	for i, b64 := range images {
		prefix := fmt.Sprintf("image_ollama_%d", time.Now().Unix())
		filename, err := service.SaveBase64Image(shared.OutputDir, prefix, b64, i)
		if err != nil {
			return saved, fmt.Errorf("failed to save image %d: %w", i, err)
		}
		saved = append(saved, filename)
	}
	return saved, nil
}

// runOllamaImage handles image generation via Ollama's native /api/generate endpoint.
func runOllamaImage(c client.APIClient, req *types.GenerateRequest, _ *imageDispatchCtx) ([]string, error) {
	start := time.Now()
	saved, err := ollamaGenerateImages(c.BaseURL(), req)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Model: %s\n", req.Model)
	fmt.Printf("Duration: %.1fs\n", time.Since(start).Seconds())
	for i, f := range saved {
		fmt.Printf("Image %d: %s\n", i+1, f)
	}
	postProcessImages(saved)
	return saved, nil
}
