// Package balance implements the `aigc-cli balance` command.
package balance

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// Deps carries the runtime configuration the balance command needs.
type Deps struct {
	ProviderSet     bool
	Provider        string
	ResolveProvider func(string) *provider.EffectiveProvider
	Providers       map[string]*types.NamedProvider
}

// Query renders the balance for one provider. scope is "all" (default; every
// endpoint the provider exposes), "token" (API-key scope) or "user" (account
// scope). Providers with a single endpoint show it regardless of scope.
func Query(p *provider.EffectiveProvider, scope string) string {
	label := providerLabel(p)

	if p.ProviderType == provider.ModelScope {
		return fmt.Sprintf("Token Balance (%s):\n  API-Inference is free. Quota info shown after each image generation.", label)
	}

	c := client.NewFromProvider(p)

	if sections, ok := providerSections(c, p, label); ok {
		return renderSections(sections, scope)
	}

	return genericBalance(c, label, scope)
}

// GetText queries balance for all configured providers.
func GetText(d Deps, scope string) (string, error) {
	if d.ProviderSet && d.Provider != "" {
		if err := options.RequireAPIKey("balance", resolveGlobal(d), d.Providers); err != nil {
			return "", err
		}
	}
	providers := collectProviders(d)
	var results []string
	for _, p := range providers {
		results = append(results, Query(p, scope))
	}
	if len(results) == 0 {
		if err := options.RequireAPIKey("balance", resolveGlobal(d), d.Providers); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no providers configured")
	}
	return strings.Join(results, "\n\n"), nil
}

func resolveGlobal(d Deps) *provider.EffectiveProvider {
	if d.ResolveProvider == nil {
		return nil
	}
	return d.ResolveProvider("balance")
}

func collectProviders(d Deps) []*provider.EffectiveProvider {
	if d.ProviderSet && d.Provider != "" {
		return []*provider.EffectiveProvider{d.ResolveProvider("balance")}
	}

	if d.Providers == nil {
		return nil
	}

	var result []*provider.EffectiveProvider
	seen := map[string]bool{}

	for name := range d.Providers {
		if seen[name] {
			continue
		}
		seen[name] = true
		ep := provider.ResolveCmdProvider(nil, name, d.Providers, &provider.GlobalConfig{})
		if ep == nil {
			continue
		}
		if ep.Type == types.ProviderLocal || (ep.Type == types.ProviderOllama && provider.IsLocalEndpoint(ep.BaseURL)) {
			continue
		}
		if ep.APIKey == "" {
			continue
		}
		result = append(result, ep)
	}

	if !seen["apimart"] {
		global := d.ResolveProvider("balance")
		if global != nil && global.APIKey != "" {
			result = append(result, global)
		}
	}

	return result
}

func providerLabel(p *provider.EffectiveProvider) string {
	if p.Name != "" {
		return p.Name
	}
	return p.ProviderType.String()
}

// NewCommand builds the `balance` command tree. deps is resolved at run time.
func NewCommand(deps func() Deps) *cobra.Command {
	balanceCmd := &cobra.Command{
		Use:          "balance [token|user]",
		Short:        "Query API key or user account balance",
		SilenceUsage: true,
		Long: `Query balance information.

Supported providers with a balance API: APIMart (and compatible relays),
OpenRouter, DeepSeek, Moonshot (Kimi), OfoxAI, SoMark (remaining page quota),
and relays built on new-api/one-api (aibase, agnes, openlux, ...), which expose
spend rather than a remaining balance. Other providers have no public balance
API — check their web console.

By default every balance endpoint a provider exposes is queried and shown.
Providers with more than one: APIMart (key quota + account balance) and
OpenRouter (key usage + account credits; the latter needs a management key).

Accepts --provider flag to query a specific named provider's balance.
Without --provider, queries every configured provider that has an API key.

Subcommands (narrow the output to one scope):
  balance token   - Only the API key (token) scope
  balance user    - Only the account (user) scope

Examples:
  aigc-cli balance
  aigc-cli balance --provider openrouter
  aigc-cli balance token
  aigc-cli balance user`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printBalance(deps(), "all")
		},
	}

	balanceTokenCmd := &cobra.Command{
		Use:          "token",
		Short:        "Query the API key (token) scope only",
		SilenceUsage: true,
		Example:      `  aigc-cli balance token`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printBalance(deps(), "token")
		},
	}

	balanceUserCmd := &cobra.Command{
		Use:          "user",
		Short:        "Query the account (user) scope only",
		SilenceUsage: true,
		Example: `  aigc-cli balance user
  aigc-cli balance user --provider apimart`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printBalance(deps(), "user")
		},
	}
	balanceCmd.AddCommand(balanceTokenCmd, balanceUserCmd)
	return balanceCmd
}

func printBalance(d Deps, scope string) error {
	text, err := GetText(d, scope)
	if err != nil {
		return err
	}
	fmt.Println(text)
	return nil
}
