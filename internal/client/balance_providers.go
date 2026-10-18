package client

import (
	"net/url"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// Provider-specific balance endpoints. APIMart's token/user balance lives in
// client_generate.go; these cover providers whose balance API differs.

// GetOpenRouterCredits returns account-level credits from GET {base}/credits.
// Requires a management key; a regular inference key receives 403.
func (c *Client) GetOpenRouterCredits() (*types.OpenRouterCredits, error) {
	var result types.OpenRouterCredits
	if err := c.doGet("/credits", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOpenRouterKey returns the current API key's info from GET {base}/key
// (limit, usage, limit_remaining).
func (c *Client) GetOpenRouterKey() (*types.OpenRouterKeyInfo, error) {
	var result types.OpenRouterKeyInfo
	if err := c.doGet("/key", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDeepSeekBalance returns the account balance from GET {root}/user/balance.
// DeepSeek's balance endpoint lives at the host root (no /v1 prefix), so it is
// fetched with an absolute URL.
func (c *Client) GetDeepSeekBalance() (*types.DeepSeekBalance, error) {
	var result types.DeepSeekBalance
	if err := c.doGetAbsolute(rootURL(c.baseURL)+"/user/balance", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetMoonshotBalance returns the account balance from
// GET {base}/users/me/balance. Amounts are in CNY.
func (c *Client) GetMoonshotBalance() (*types.MoonshotBalance, error) {
	var result types.MoonshotBalance
	if err := c.doGet("/users/me/balance", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// rootURL returns scheme://host of an absolute URL (no path), for endpoints that
// live outside the API version prefix. Falls back to the trimmed input when the
// URL cannot be parsed.
func rootURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return strings.TrimRight(rawURL, "/")
	}
	return u.Scheme + "://" + u.Host
}
