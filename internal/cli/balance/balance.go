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

func queryOne(p *provider.EffectiveProvider, scope string) string {
	label := providerLabel(p)

	if p.ProviderType == provider.ModelScope {
		return fmt.Sprintf("Token Balance (%s):\n  API-Inference is free. Quota info shown after each image generation.", label)
	}

	c := client.NewFromProvider(p)

	if scope == "user" {
		bal, err := c.GetUserBalance()
		if err != nil {
			return fmt.Sprintf("User Balance (%s): error — %v", label, err)
		}
		if !bal.Success {
			return fmt.Sprintf("User Balance (%s): API error — %s", label, bal.Message)
		}
		return fmt.Sprintf("User Balance (%s):\n  Remain Balance: $%.4f\n  Remain Credits: %.4f\n  Used Balance: $%.4f\n  Used Credits: %.4f",
			label, bal.RemainBalance, bal.RemainCredits, bal.UsedBalance, bal.UsedCredits)
	}

	bal, err := c.GetTokenBalance()
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return fmt.Sprintf("Token Balance (%s): not available\n  Check your balance on the provider's web console.", label)
		}
		return fmt.Sprintf("Token Balance (%s): error — %v", label, err)
	}
	if !bal.Success {
		return fmt.Sprintf("Token Balance (%s): API error — %s", label, bal.Message)
	}
	msg := fmt.Sprintf("Token Balance (%s):\n", label)
	if bal.UnlimitedQuota {
		msg += "  Status: Unlimited Quota (no limit)\n"
	} else {
		msg += fmt.Sprintf("  Remain Balance: $%.4f\n  Remain Credits: %.4f\n", bal.RemainBalance, bal.RemainCredits)
	}
	msg += fmt.Sprintf("  Used Balance: $%.4f\n  Used Credits: %.4f", bal.UsedBalance, bal.UsedCredits)
	return msg
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
		results = append(results, queryOne(p, scope))
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

Accepts --provider flag to query a specific named provider's balance.
Without --provider, queries every configured provider that has an API key.

Subcommands:
  balance token   - Query the current API key (token) balance (default)
  balance user    - Query the entire user account balance

If no subcommand is given, defaults to "token".

Examples:
  aigc-cli balance
  aigc-cli balance --provider siliconflow
  aigc-cli balance token
  aigc-cli balance user`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printBalance(deps(), "token")
		},
	}

	balanceUserCmd := &cobra.Command{
		Use:          "user",
		Short:        "Query user account balance",
		SilenceUsage: true,
		Example: `  aigc-cli balance user
  aigc-cli balance user --provider siliconflow`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printBalance(deps(), "user")
		},
	}
	balanceCmd.AddCommand(balanceUserCmd)
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
