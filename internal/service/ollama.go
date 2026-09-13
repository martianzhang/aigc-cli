package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

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

// OllamaGenerateImages sends a request to Ollama's /api/generate and returns
// the saved image filenames under outputDir.
func OllamaGenerateImages(baseURL string, req *types.GenerateRequest, outputDir string) ([]string, error) {
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
		filename, err := SaveBase64Image(outputDir, prefix, b64, i)
		if err != nil {
			return saved, fmt.Errorf("failed to save image %d: %w", i, err)
		}
		saved = append(saved, filename)
	}
	return saved, nil
}
