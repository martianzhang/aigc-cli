package redact

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/terminal"
)

func newCheckCmd() *cobra.Command {
	var mappingFile string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "check [file|-]",
		Short: "Scan a redacted file for leftover original values",
		Example: `  # Exit with code 2 when a leftover original value is found
  aigc-cli redact check contract.redacted.txt --mapping contract.map.json

  # JSON output for CI or further processing
  aigc-cli redact check contract.redacted.txt --mapping contract.map.json --json

  # Check stdin
  cat contract.redacted.txt | aigc-cli redact check - --mapping contract.map.json`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loadDeps()
			if mappingFile == "" {
				return fmt.Errorf("--mapping is required")
			}
			input := "-"
			if len(args) > 0 {
				input = args[0]
			}
			_, text, err := readInput(input)
			if err != nil {
				return err
			}
			m, err := redactlib.LoadMapping(mappingFile)
			if err != nil {
				return err
			}
			residuals := redactlib.Check(text, m)
			if asJSON {
				out := struct {
					Count     int                  `json:"count"`
					Residuals []redactlib.Residual `json:"residuals"`
				}{Count: len(residuals), Residuals: residuals}
				data, err := json.MarshalIndent(out, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				headers := []string{"TYPE", "VALUE", "POS"}
				rows := make([][]string, 0, len(residuals))
				for _, r := range residuals {
					rows = append(rows, []string{r.Type, r.Value, fmt.Sprintf("%d..%d", r.Start, r.End)})
				}
				_ = terminal.WriteTable(cmd.OutOrStdout(), headers, rows, terminal.TableOptions{})
				fmt.Fprintf(cmd.OutOrStdout(), "\nresidual: %d\n", len(residuals))
			}
			if len(residuals) > 0 {
				return &redactlib.ResidualError{Count: len(residuals)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mappingFile, "mapping", "", "Mapping file written by redact apply --mapping")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output machine-readable JSON")
	return cmd
}
