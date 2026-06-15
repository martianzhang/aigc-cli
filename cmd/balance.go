package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/client"
)

// balanceCmd represents the `balance` command.
var balanceCmd = &cobra.Command{
	Use:   "balance [token|user]",
	Short: "Query API key or user account balance",
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
	Use:   "user",
	Short: "Query user account balance",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBalanceUser()
	},
}

func runBalanceToken() error {
	c := client.New(apiKey, apiBase, httpProxy)
	bal, err := c.GetTokenBalance()
	if err != nil {
		return fmt.Errorf("failed to query token balance: %w", err)
	}

	if !bal.Success {
		return fmt.Errorf("API error: %s", bal.Message)
	}

	fmt.Println("Token Balance:")
	if bal.UnlimitedQuota {
		fmt.Println("  Status:         Unlimited Quota (no limit)")
	} else {
		fmt.Printf("  Remain Balance: $%.4f\n", bal.RemainBalance)
		fmt.Printf("  Remain Credits: %.4f\n", bal.RemainCredits)
	}
	fmt.Printf("  Used Balance:   $%.4f\n", bal.UsedBalance)
	fmt.Printf("  Used Credits:   %.4f\n", bal.UsedCredits)
	return nil
}

func runBalanceUser() error {
	c := client.New(apiKey, apiBase, httpProxy)
	bal, err := c.GetUserBalance()
	if err != nil {
		return fmt.Errorf("failed to query user balance: %w", err)
	}

	if !bal.Success {
		return fmt.Errorf("API error: %s", bal.Message)
	}

	fmt.Println("User Balance:")
	fmt.Printf("  Remain Balance: $%.4f\n", bal.RemainBalance)
	fmt.Printf("  Remain Credits: %.4f\n", bal.RemainCredits)
	fmt.Printf("  Used Balance:   $%.4f\n", bal.UsedBalance)
	fmt.Printf("  Used Credits:   %.4f\n", bal.UsedCredits)
	return nil
}

func init() {
	rootCmd.AddCommand(balanceCmd)
	balanceCmd.AddCommand(balanceUserCmd)
}
