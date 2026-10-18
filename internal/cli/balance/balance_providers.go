package balance

import (
	"errors"
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
)

// balanceSection is one labeled balance endpoint result. kind is "token"
// (API-key scope) or "user" (account scope); a provider may expose either or
// both.
type balanceSection struct {
	kind string
	text string
}

// providerSections returns every balance section a known provider exposes, and
// false when baseURL matches none (the caller then runs the generic probe).
func providerSections(c *client.Client, p *provider.EffectiveProvider, label string) ([]balanceSection, bool) {
	switch {
	case provider.IsOpenRouter(p.BaseURL):
		return openRouterSections(c, label), true
	case provider.IsDeepSeek(p.BaseURL):
		return singleSection("user", deepSeekBalance(c, label)), true
	case provider.IsMoonshot(p.BaseURL):
		return singleSection("user", moonshotBalance(c, label)), true
	case p.ProviderType == provider.APIMart || provider.IsAPIMart(p.BaseURL):
		return apimartSections(c, label), true
	}
	return nil, false
}

func singleSection(kind, text string) []balanceSection {
	return []balanceSection{{kind: kind, text: text}}
}

// renderSections renders the requested scope. "all" (or "") shows every
// section; "token"/"user" narrows to that kind, falling back to all when the
// provider exposes no section of the requested kind.
func renderSections(sections []balanceSection, scope string) string {
	chosen := sections
	if scope != "" && scope != "all" {
		var filtered []balanceSection
		for _, s := range sections {
			if s.kind == scope {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) > 0 {
			chosen = filtered
		}
	}
	texts := make([]string, 0, len(chosen))
	for _, s := range chosen {
		texts = append(texts, s.text)
	}
	return strings.Join(texts, "\n\n")
}

// apimartSections renders APIMart's two endpoints: GET /balance (key quota) and
// GET /user/balance (account balance).
func apimartSections(c *client.Client, label string) []balanceSection {
	return []balanceSection{
		{kind: "token", text: apimartTokenBalance(c, label)},
		{kind: "user", text: apimartUserBalance(c, label)},
	}
}

// apimartTokenBalance renders GET /balance (per-key quota). It is also the
// generic probe for providers without a dedicated balance API.
func apimartTokenBalance(c *client.Client, label string) string {
	bal, err := c.GetTokenBalance()
	if err != nil {
		return balanceError("Token Balance", label, err)
	}
	if !bal.Success {
		return fmt.Sprintf("Token Balance (%s): API error — %s", label, bal.Message)
	}
	var lines []string
	if bal.UnlimitedQuota {
		lines = append(lines, "Status: Unlimited Quota (no limit)")
	} else {
		lines = append(lines, fmt.Sprintf("Remain Balance: $%.4f", bal.RemainBalance))
		lines = append(lines, fmt.Sprintf("Remain Credits: %.4f", bal.RemainCredits))
	}
	lines = append(lines, fmt.Sprintf("Used Balance: $%.4f", bal.UsedBalance))
	lines = append(lines, fmt.Sprintf("Used Credits: %.4f", bal.UsedCredits))
	return titledBlock("Token Balance", label, lines)
}

// apimartUserBalance renders GET /user/balance (account balance).
func apimartUserBalance(c *client.Client, label string) string {
	bal, err := c.GetUserBalance()
	if err != nil {
		return balanceError("User Balance", label, err)
	}
	if !bal.Success {
		return fmt.Sprintf("User Balance (%s): API error — %s", label, bal.Message)
	}
	return titledBlock("User Balance", label, []string{
		fmt.Sprintf("Remain Balance: $%.4f", bal.RemainBalance),
		fmt.Sprintf("Remain Credits: %.4f", bal.RemainCredits),
		fmt.Sprintf("Used Balance: $%.4f", bal.UsedBalance),
		fmt.Sprintf("Used Credits: %.4f", bal.UsedCredits),
	})
}

// openRouterSections renders GET /key (key scope, any key) and, when the
// configured key is a management key, GET /credits (account scope).
func openRouterSections(c *client.Client, label string) []balanceSection {
	var sections []balanceSection

	credits, creditsErr := c.GetOpenRouterCredits()
	if creditsErr == nil {
		d := credits.Data
		sections = append(sections, balanceSection{kind: "user", text: titledBlock("Account Balance", label, []string{
			fmt.Sprintf("Remain: $%.4f (credits)", d.TotalCredits-d.TotalUsage),
			fmt.Sprintf("Used: $%.4f", d.TotalUsage),
			fmt.Sprintf("Purchased: $%.4f", d.TotalCredits),
		})})
	}

	info, keyErr := c.GetOpenRouterKey()
	if keyErr != nil {
		if creditsErr != nil {
			return singleSection("token", balanceError("Balance", label, keyErr))
		}
		return sections
	}
	d := info.Data
	var lines []string
	if d.IsFreeTier {
		lines = append(lines, "Tier: free")
	}
	lines = append(lines, fmt.Sprintf("Used: $%.4f", d.Usage))
	if d.Limit == nil {
		lines = append(lines, "Limit: none")
	} else {
		lines = append(lines, fmt.Sprintf("Limit: $%.4f", *d.Limit))
		if d.LimitRemaining != nil {
			lines = append(lines, fmt.Sprintf("Remain: $%.4f", *d.LimitRemaining))
		}
	}
	if creditsErr != nil {
		lines = append(lines, "(account credits need a management key)")
	}
	sections = append(sections, balanceSection{kind: "token", text: titledBlock("Key Balance", label, lines)})
	return sections
}

// deepSeekBalance renders GET {root}/user/balance (one entry per currency).
func deepSeekBalance(c *client.Client, label string) string {
	bal, err := c.GetDeepSeekBalance()
	if err != nil {
		return balanceError("Balance", label, err)
	}
	var lines []string
	if !bal.IsAvailable {
		lines = append(lines, "Status: insufficient balance")
	}
	if len(bal.BalanceInfos) == 0 {
		lines = append(lines, "No balance info returned")
	}
	for _, b := range bal.BalanceInfos {
		lines = append(lines, fmt.Sprintf("%s: total %s (granted %s, topped-up %s)",
			b.Currency, b.TotalBalance, b.GrantedBalance, b.ToppedUpBalance))
	}
	return titledBlock("Balance", label, lines)
}

// moonshotBalance renders GET {base}/users/me/balance (amounts in CNY).
func moonshotBalance(c *client.Client, label string) string {
	bal, err := c.GetMoonshotBalance()
	if err != nil {
		return balanceError("Balance", label, err)
	}
	d := bal.Data
	return titledBlock("Balance", label, []string{
		fmt.Sprintf("Available: ¥%.5f", d.AvailableBalance),
		fmt.Sprintf("Cash: ¥%.5f", d.CashBalance),
		fmt.Sprintf("Voucher: ¥%.5f", d.VoucherBalance),
	})
}

// titledBlock renders "<title> (<label>):" with two-space-indented lines.
func titledBlock(title, label string, lines []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s):", title, label)
	for _, ln := range lines {
		fmt.Fprintf(&b, "\n  %s", ln)
	}
	return b.String()
}

// balanceError renders a provider error, degrading a missing endpoint to a
// console pointer instead of an opaque failure.
func balanceError(kind, label string, err error) string {
	if errors.Is(err, client.ErrNotSupported) || strings.Contains(err.Error(), "404") {
		return fmt.Sprintf("%s (%s): not available\n  Check your balance on the provider's web console.", kind, label)
	}
	return fmt.Sprintf("%s (%s): error — %v", kind, label, err)
}
