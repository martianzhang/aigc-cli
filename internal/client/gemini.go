package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

const geminiInteractionsPath = "/interactions"

// geminiInteractionsRequest is the request body for Gemini Interactions API.
type geminiInteractionsRequest struct {
	Model          string                `json:"model"`
	Input          interface{}           `json:"input"`
	ResponseFormat *geminiResponseFormat `json:"response_format,omitempty"`
}

// geminiResponseFormat specifies the output format.
type geminiResponseFormat struct {
	Type        string `json:"type,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	ImageSize   string `json:"image_size,omitempty"`
}

// geminiInputItem represents a text or image input.
type geminiInputItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// geminiInteractionsResponse is the response from Gemini Interactions API.
type geminiInteractionsResponse struct {
	ID    string       `json:"id"`
	Steps []geminiStep `json:"steps"`
}

type geminiStep struct {
	Type    string          `json:"type"`
	Content []geminiContent `json:"content,omitempty"`
}

type geminiContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"`
}

type geminiErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// GeminiImageGenerate generates images using Gemini Interactions API.
func (c *Client) GeminiImageGenerate(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	input := []geminiInputItem{
		{Type: "text", Text: req.Prompt},
	}

	respFormat := &geminiResponseFormat{Type: "image"}

	if req.Size != "" {
		parts := strings.Split(req.Size, "x")
		if len(parts) == 2 {
			w, h := parts[0], parts[1]
			respFormat.AspectRatio = simplifyRatio(w, h)
		}
	}

	geminiReq := &geminiInteractionsRequest{
		Model:          req.Model,
		Input:          input,
		ResponseFormat: respFormat,
	}

	body, err := c.doGeminiRequest(http.MethodPost, geminiInteractionsPath, geminiReq)
	if err != nil {
		return nil, err
	}

	return c.parseGeminiResponse(body)
}

func (c *Client) doGeminiRequest(method, path string, reqBody interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := strings.TrimRight(c.baseURL, "/") + path
	httpReq, err := http.NewRequestWithContext(c.requestContext(), method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Gemini API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp geminiErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("Gemini API error (%s): %s", errResp.Error.Status, errResp.Error.Message)
		}
		return nil, fmt.Errorf("Gemini API returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) parseGeminiResponse(body []byte) (*types.OpenAIImageResponse, error) {
	var geminiResp geminiInteractionsResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini response: %w", err)
	}

	var data []types.OpenAIImageData
	for _, step := range geminiResp.Steps {
		if step.Type == "model_output" {
			for _, content := range step.Content {
				if content.Type == "image" && content.Data != "" {
					data = append(data, types.OpenAIImageData{
						URL: "data:image/png;base64," + content.Data,
					})
				}
			}
		}
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("Gemini returned no images")
	}

	return &types.OpenAIImageResponse{Data: data}, nil
}

func simplifyRatio(w, h string) string {
	ratios := map[string]string{
		"1024:1024": "1:1", "2048:2048": "1:1",
		"1024:1536": "2:3", "1536:1024": "3:2",
		"1024:1365": "3:4", "1365:1024": "4:3",
		"1024:1280": "4:5", "1280:1024": "5:4",
		"1024:1820": "9:16", "1820:1024": "16:9",
		"1024:2401": "21:9", "2401:1024": "9:21",
	}
	key := w + ":" + h
	if r, ok := ratios[key]; ok {
		return r
	}
	return "1:1"
}
