package redact

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/terminal"
)

func newRulesCmd() *cobra.Command {
	var asJSON bool
	var export bool
	var flags buildFlags
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List the effective rule set (built-ins + overrides + custom)",
		Example: `  # Show the effective rule set
  aigc-cli redact rules

  # Export for review or version control
  aigc-cli redact rules --json > rules.json
  aigc-cli redact rules --export > rules.yaml

  # Inspect an external profile, or toggle individual rules
  aigc-cli redact rules --profile strict
  aigc-cli redact rules --enable passport,lawyer_no
  aigc-cli redact rules --disable bank_card,plate
  aigc-cli redact rules --rules-file team-rules.yaml`,
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			loadDeps()
			cfg, err := flags.redactConfig()
			if err != nil {
				return err
			}
			if err := validateRuleFlags(cfg, flags); err != nil {
				return err
			}
			engine, err := redactlib.NewRuleEngine(cfg)
			if err != nil {
				return err
			}
			rules := engine.Rules()
			if asJSON {
				data, err := json.MarshalIndent(rules, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}
			if export {
				data, err := yaml.Marshal(rules)
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), string(data))
				return nil
			}
			return writeRulesTable(cmd.OutOrStdout(), rules)
		},
	}
	addRuleFlags(cmd, &flags)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output JSON")
	cmd.Flags().BoolVar(&export, "export", false, "Export the effective rules as YAML (informational)")
	return cmd
}

// writeRulesTable aligns columns by display width (CJK characters are two
// cells wide), which text/tabwriter cannot do.
func writeRulesTable(w io.Writer, rules []redactlib.RuleInfo) error {
	headers := []string{"NAME", "CATEGORY", "TYPE", "ON", "PRI", "REGEX"}
	rows := make([][]string, 0, len(rules))
	for _, r := range rules {
		on := "no"
		if r.Enabled {
			on = "yes"
		}
		rows = append(rows, []string{r.Name, r.Category, r.Type, on, fmt.Sprintf("%d", r.Priority), r.Regex})
	}
	return terminal.WriteTable(w, headers, rows, terminal.TableOptions{})
}
