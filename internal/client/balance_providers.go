package client

import (
	"fmt"
	"net/url"
	"strings"
	"time"

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

// GetOfoxBalance returns the account balance from GET {base}/user/balance.
// OfoxAI documents it as cc-switch compatible, so it is a real balance API
// rather than a usage report.
func (c *Client) GetOfoxBalance() (*types.OfoxBalance, error) {
	var result types.OfoxBalance
	if err := c.doGet("/user/balance", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOpenAIBilling queries the OpenAI-compatible billing pair:
// GET {base}/dashboard/billing/subscription and
// GET {base}/dashboard/billing/usage over the last 30 days. Relay platforms
// built on new-api/one-api expose both without a session, which is how they
// report account spend. Whichever endpoint answers is returned; both nil means
// the provider has no billing API.
func (c *Client) GetOpenAIBilling() (*types.OpenAIBillingSubscription, *types.OpenAIBillingUsage, error) {
	var subscription types.OpenAIBillingSubscription
	subscriptionErr := c.doGet("/dashboard/billing/subscription", &subscription)

	end := time.Now().UTC()
	start := end.AddDate(0, 0, -30)
	usagePath := fmt.Sprintf("/dashboard/billing/usage?start_date=%s&end_date=%s",
		start.Format("2006-01-02"), end.Format("2006-01-02"))
	var usage types.OpenAIBillingUsage
	usageErr := c.doGet(usagePath, &usage)

	if subscriptionErr != nil && usageErr != nil {
		return nil, nil, subscriptionErr
	}
	if subscriptionErr != nil {
		return nil, &usage, nil
	}
	if usageErr != nil {
		return &subscription, nil, nil
	}
	return &subscription, &usage, nil
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
