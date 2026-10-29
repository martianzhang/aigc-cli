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

// GeminiInteractionsPath is Gemini's native Interactions endpoint.
const GeminiInteractionsPath = "/interactions"

// geminiInteractionsRequest is the request body for Gemini Interactions API.
type geminiInteractionsRequest struct {
	Model          string                `json:"model"`
	Input          any                   `json:"input"`
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
	URI      string `json:"uri,omitempty"`
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

// GeminiImageBody builds the body for Gemini's Interactions image API. A
// verbatim --json body wins; otherwise the typed fields are mapped onto
// input/response_format. Shared by the real request and --dry-run/--verbose so
// the preview cannot drift from what is sent.
func GeminiImageBody(req *types.GenerateRequest) (any, error) {
	input := []geminiInputItem{
		{Type: "text", Text: req.Prompt},
	}
	for _, ref := range req.ImageURLs {
		input = append(input, geminiImageItem(ref))
	}

	aspectRatio, imageSize := geminiImageFormat(req.Size, req.Ratio)
	respFormat := &geminiResponseFormat{
		Type:        "image",
		AspectRatio: aspectRatio,
		ImageSize:   imageSize,
	}

	return req.BodyOrRaw(&geminiInteractionsRequest{
		Model:          req.Model,
		Input:          input,
		ResponseFormat: respFormat,
	})
}

// geminiImageItem maps one reference image onto the Interactions input shape. A
// data URI (data:<mime>;base64,<b64>) becomes raw base64 in data; anything else
// (public https URL or opaque handle) is passed through as uri.
func geminiImageItem(value string) geminiInputItem {
	if strings.HasPrefix(value, "data:") {
		header, payload, ok := strings.Cut(value, ",")
		if ok && strings.HasSuffix(header, ";base64") {
			mimeType := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
			return geminiInputItem{Type: "image", Data: payload, MimeType: mimeType}
		}
	}
	return geminiInputItem{Type: "image", URI: value}
}

// GeminiImageGenerate generates images using Gemini Interactions API.
func (c *Client) GeminiImageGenerate(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	reqBody, err := GeminiImageBody(req)
	if err != nil {
		return nil, err
	}
	body, err := c.doGeminiRequest(http.MethodPost, GeminiInteractionsPath, reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseGeminiResponse(body)
}

func (c *Client) doGeminiRequest(method, path string, reqBody any) ([]byte, error) {
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
		return nil, fmt.Errorf("gemini API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp geminiErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("gemini API error (%s): %s", errResp.Error.Status, errResp.Error.Message)
		}
		return nil, fmt.Errorf("gemini API returned status %d: %s", resp.StatusCode, string(body))
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
		return nil, fmt.Errorf("gemini returned no images")
	}

	return &types.OpenAIImageResponse{Data: data}, nil
}
