package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// balanceCmd represents the `balance` command.
var balanceCmd = &cobra.Command{
	Use:          "balance [token|user]",
	Short:        "Query API key or user account balance",
	SilenceUsage: true,
	Long: `Query balance information.

Accepts --provider flag to query a specific named provider's balance.
Uses the global/default provider if --provider is not set.

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
		// Default to token balance if no subcommand matched
		return runBalanceToken()
	},
}

// balanceUserCmd represents the `balance user` subcommand.
var balanceUserCmd = &cobra.Command{
	Use:          "user",
	Short:        "Query user account balance",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBalanceUser()
	},
}

func queryOneBalance(p *provider.EffectiveProvider, scope string) string {
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

// getBalanceText queries balance for all providers.
// When --provider is set, only queries that one; otherwise queries all named providers.
func getBalanceText(scope string) (string, error) {
	providers := collectBalanceProviders()
	var results []string
	for _, p := range providers {
		results = append(results, queryOneBalance(p, scope))
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no providers configured")
	}
	return strings.Join(results, "\n\n"), nil
}

func collectBalanceProviders() []*provider.EffectiveProvider {
	if shared.ProviderSet && shared.Provider != "" {
		return []*provider.EffectiveProvider{shared.ResolveProvider("balance")}
	}

	if shared.Cfg == nil || shared.Cfg.Providers == nil {
		return nil
	}

	var result []*provider.EffectiveProvider
	seen := map[string]bool{}

	for name := range shared.Cfg.Providers {
		if seen[name] {
			continue
		}
		seen[name] = true
		ep := provider.ResolveCmdProvider(nil, name, shared.Cfg.Providers, &provider.GlobalConfig{})
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
		global := shared.ResolveProvider("balance")
		if global != nil && global.APIKey != "" {
			result = append(result, global)
		}
	}

	return result
}

func runBalanceToken() error {
	text, err := getBalanceText("token")
	if err != nil {
		return err
	}
	fmt.Println(text)
	return nil
}

func runBalanceUser() error {
	text, err := getBalanceText("user")
	if err != nil {
		return err
	}
	fmt.Println(text)
	return nil
}

// providerLabel returns a display name for the effective provider.
func providerLabel(p *provider.EffectiveProvider) string {
	if p.Name != "" {
		return p.Name
	}
	return p.ProviderType.String()
}

func init() {
	rootCmd.AddCommand(balanceCmd)
	balanceCmd.AddCommand(balanceUserCmd)
}
