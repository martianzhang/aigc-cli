package client

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// GetTokenBalance queries the current token's balance.
func (c *Client) GetTokenBalance() (*types.TokenBalanceResponse, error) {
	return getBalance[types.TokenBalanceResponse](c, tokenBalancePath)
}

// GetUserBalance queries the current user's balance.
func (c *Client) GetUserBalance() (*types.UserBalanceResponse, error) {
	return getBalance[types.UserBalanceResponse](c, userBalancePath)
}

// getBalance is a generic helper for balance endpoints.
// path should be relative (without baseURL), doGet prepends it.
func getBalance[T any](c *Client, path string) (*T, error) {
	var result T
	if err := c.doGet(path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// --- Provider detection ---

// IsAPIMartProvider returns true if the base URL points to an APIMart-provided domain.
func (c *Client) IsAPIMartProvider() bool {
	return provider.IsAPIMart(c.baseURL)
}

// IsOpenRouterProvider returns true if the base URL points to OpenRouter.
func (c *Client) IsOpenRouterProvider() bool {
	return provider.IsOpenRouter(c.baseURL)
}

// --- Sync image generation (OpenAI / OpenRouter compatible) ---

// ImageGenerateSync sends a synchronous image generation request compatible with
// OpenAI and OpenRouter. Returns the response with image URLs directly.
func (c *Client) ImageGenerateSync(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	cleanReq := c.sanitizeImageRequest(req)
	var result types.OpenAIImageResponse
	if err := c.doJSON(http.MethodPost, ImageSubmitPath, cleanReq, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// sanitizeImageRequest removes provider-unsupported fields from the request.
func (c *Client) sanitizeImageRequest(req *types.GenerateRequest) *types.GenerateRequest {
	p := provider.Detect(c.baseURL)
	if p == provider.ModelScope || provider.IsGeminiDomain(c.baseURL) {
		// Gemini (native + OpenAI-compat) and ModelScope don't support
		// output_format, background, or moderation — strip them.
		// OpenAI and generic relays DO support these and get the full request.
		return &types.GenerateRequest{
			Model:          req.Model,
			Prompt:         req.Prompt,
			Size:           req.Size,
			Resolution:     req.Resolution,
			Quality:        req.Quality,
			N:              req.N,
			ImageURLs:      req.ImageURLs,
			MaskURL:        req.MaskURL,
			Style:          req.Style,
			ResponseFormat: req.ResponseFormat,
		}
	}
	return req
}

// --- Balance ---

// --- Models (OpenAI-compatible) ---

// ListModelsOpenAI fetches the model list from OpenAI-compatible /v1/models endpoint.
func (c *Client) ListModelsOpenAI() ([]types.OpenAIModel, error) {
	var result struct {
		Data []types.OpenAIModel `json:"data"`
	}
	if err := c.doGet(modelsPath, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// GetModelOpenAI fetches a single model by ID from the OpenAI-compatible /v1/models/{model} endpoint.
func (c *Client) GetModelOpenAI(modelID string) (*types.OpenAIModel, error) {
	path := modelsPath + "/" + modelID
	var result types.OpenAIModel
	if err := c.doGet(path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetModelOpenRouter fetches a single model from OpenRouter's
// GET /api/v1/model/{author}/{slug} endpoint and returns the raw JSON. The
// response schema varies (e.g. supported_parameters is an array), so it is left
// unparsed for the caller to display as-is. Note the singular "model" segment:
// OpenRouter has no OpenAI-style plural /models/{id} lookup.
func (c *Client) GetModelOpenRouter(modelID string) (json.RawMessage, error) {
	var result json.RawMessage
	if err := c.doGet(openRouterModelPath+modelID, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// --- Helpers ---

// setOpenRouterHeaders sets OpenRouter-specific headers on the request.
// Priority: env var > defaultHeaders > nothing.
// Also sets User-Agent from defaultHeaders if not already set.
func (c *Client) setOpenRouterHeaders(req *http.Request) {
	// Apply default headers first as fallback.
	for k, v := range c.defaultHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
	// Env vars override defaults (highest priority).
	if ref := os.Getenv("OPENAI_REFERER"); ref != "" {
		req.Header.Set(headerReferer, ref)
	}
	if title := os.Getenv("OPENAI_APP_TITLE"); title != "" {
		req.Header.Set(headerTitle, title)
	}
}

// hasVersionSuffix checks if urlStr ends with a version path segment like /v1, /v2, /v3.
