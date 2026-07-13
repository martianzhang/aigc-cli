package client

import (
	"context"
	"net/http"
	"time"
)

const (
	defaultBaseURL    = "https://api.apimart.ai"
	imageSubmitPath   = "/images/generations"
	videoSubmitPath   = "/videos/generations"
	yunwuVideoSubPath = "/video/create"
	yunwuVideoQryPath = "/video/query"
	chatPath          = "/chat/completions"
	uploadPath        = "/uploads/images"
	taskPath          = "/tasks/%s"
	tokenBalancePath  = "/balance"
	userBalancePath   = "/user/balance"
	modelsPath        = "/models"
	// OpenRouter-specific header names
	headerReferer = "HTTP-Referer"
	headerTitle   = "X-OpenRouter-Title"
	// Default polling settings
	pollInterval    = 3 * time.Second
	initialDelay    = 10 * time.Second
	maxPollDuration = 180 * time.Second
	uploadTimeout   = 60 * time.Second
	// Default HTTP client timeout for API requests (used as initial value for DefaultTimeout)
	defaultHTTPTimeout = 180 * time.Second
	// Modality-specific HTTP timeouts (exported for use by cmd/ commands)
	ImageTimeout = 180 * time.Second
	VideoTimeout = 600 * time.Second
	MJTimeout    = 600 * time.Second
)

// DefaultTimeout is the timeout used by New(). Commands can override this
// before creating clients (safe for single-threaded CLI usage).
var DefaultTimeout = defaultHTTPTimeout

// Client is the API client for image generation, chat, and more.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	ctx        context.Context // optional, for cancellation (set by SetContext)
}

// SetContext sets an optional context for cancellation support.
// When set, all HTTP requests and polling loops respect ctx.Done().
// Thread-safe for single-threaded CLI usage.
func (c *Client) SetContext(ctx context.Context) {
	c.ctx = ctx
}

// requestContext returns the client's context or context.Background().
func (c *Client) requestContext() context.Context {
	return c.GetContext()
}

// GetContext returns the client's context or context.Background().
// Exported so cmd/ packages can propagate context to new clients.
func (c *Client) GetContext() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

// SetTimeout sets the HTTP client timeout. Use 0 for no timeout.
func (c *Client) SetTimeout(d time.Duration) {
	c.httpClient.Timeout = d
}

// New creates a new API client.
// Pass empty strings for baseURL or proxyURL to use defaults.
// proxyURL supports http://, https://, socks5:// schemes.
// When proxyURL is empty, falls back to HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY env vars.
//
// baseURL should include the API version prefix (e.g. "https://api.apimart.ai/v1").
// For backward compatibility, if baseURL doesn't end with a "/vN" path segment,
// "/v1" is appended automatically (so "https://api.openai.com" → "https://api.openai.com/v1").
// If it already includes a version (e.g. "https://relay.com/v2", "https://openrouter.ai/api/v1"),
