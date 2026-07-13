package client

import (
	"github.com/martianzhang/apimart-cli/internal/provider"
	"github.com/martianzhang/apimart-cli/internal/types"
	"net/http"
	"os"
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
	var result types.OpenAIImageResponse
	if err := c.doJSON(http.MethodPost, imageSubmitPath, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
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

// --- Helpers ---

// setOpenRouterHeaders adds optional OpenRouter-specific headers.
// Reads environment variables directly to avoid storing provider-specific state.
func (c *Client) setOpenRouterHeaders(req *http.Request) {
	if ref := os.Getenv("OPENAI_REFERER"); ref != "" {
		req.Header.Set(headerReferer, ref)
	}
	if title := os.Getenv("OPENAI_APP_TITLE"); title != "" {
		req.Header.Set(headerTitle, title)
	}
}

// hasVersionSuffix checks if urlStr ends with a version path segment like /v1, /v2, /v3.
