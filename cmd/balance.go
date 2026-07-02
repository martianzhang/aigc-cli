package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
)

// balanceCmd represents the `balance` command.
var balanceCmd = &cobra.Command{
	Use:          "balance [token|user]",
	Short:        "Query API key or user account balance",
	SilenceUsage: true,
	Long: `Query balance information.

Subcommands:
  balance token   - Query the current API key (token) balance (default)
  balance user    - Query the entire user account balance

If no subcommand is given, defaults to "token".

Examples:
  apimart-cli balance
  apimart-cli balance token
  apimart-cli balance user`,
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

// getBalanceText queries the balance and returns a human-readable summary.
// Shared by CLI (balance command) and agent loop (chat) — single source of truth.
func getBalanceText(scope string) (string, error) {
	c := client.New(shared.APIKey, shared.APIBase, shared.HTTPProxy)
	if scope == "user" {
		bal, err := c.GetUserBalance()
		if err != nil {
			return "", fmt.Errorf("failed to query user balance: %w", err)
		}
		if !bal.Success {
			return "", fmt.Errorf("API error: %s", bal.Message)
		}
		return fmt.Sprintf("User Balance:\n  Remain Balance: $%.4f\n  Remain Credits: %.4f\n  Used Balance: $%.4f\n  Used Credits: %.4f",
			bal.RemainBalance, bal.RemainCredits, bal.UsedBalance, bal.UsedCredits), nil
	}

	bal, err := c.GetTokenBalance()
	if err != nil {
		return "", fmt.Errorf("failed to query token balance: %w", err)
	}
	if !bal.Success {
		return "", fmt.Errorf("API error: %s", bal.Message)
	}
	msg := "Token Balance:\n"
	if bal.UnlimitedQuota {
		msg += "  Status: Unlimited Quota (no limit)\n"
	} else {
		msg += fmt.Sprintf("  Remain Balance: $%.4f\n  Remain Credits: %.4f\n", bal.RemainBalance, bal.RemainCredits)
	}
	msg += fmt.Sprintf("  Used Balance: $%.4f\n  Used Credits: %.4f", bal.UsedBalance, bal.UsedCredits)
	return msg, nil
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

func init() {
	rootCmd.AddCommand(balanceCmd)
	balanceCmd.AddCommand(balanceUserCmd)
}
