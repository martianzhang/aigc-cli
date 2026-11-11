package redact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/docredact"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

func newRestoreCmd() *cobra.Command {
	var stdout bool
	var mappingFile string
	var force bool
	cmd := &cobra.Command{
		Use:   "restore [file|-]",
		Short: "Restore placeholders using a mapping file",
		Example: `  # Restore next to the input as contract.restored.txt
  aigc-cli redact restore contract.redacted.txt --mapping contract.map.json

  # Print to stdout
  aigc-cli redact restore contract.redacted.txt --stdout --mapping contract.map.json

  # Read from stdin
  cat contract.redacted.txt | aigc-cli redact restore - --mapping contract.map.json

  # Restore a DOCX produced with style=placeholder
  aigc-cli redact restore contract.redacted.docx --mapping contract.map.json

  # Restore even if the placeholder or sha256 check fails (review first)
  aigc-cli redact restore contract.redacted.txt --mapping contract.map.json --force`,
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
			if input != "-" {
				prepared, outExt, cleanup, perr := prepareRedactInput(input)
				if perr != nil {
					return perr
				}
				defer cleanup()
				format, ferr := docredact.Detect(prepared)
				if ferr != nil && docredact.IsDocumentPath(input) {
					return ferr
				}
				if ferr == nil {
					if format != docredact.FormatDOCX {
						return fmt.Errorf("restore is not supported for %s input", format)
					}
					if stdout {
						return fmt.Errorf("--stdout is only supported for text input")
					}
					m, err := redactlib.LoadMapping(mappingFile)
					if err != nil {
						return err
					}
					target := defaultRestoreOut(input)
					if outExt != "" && outExt != filepath.Ext(input) {
						target = strings.TrimSuffix(target, filepath.Ext(target)) + outExt
					}
					if err := ensureNotInput(input, target); err != nil {
						return err
					}
					if err := docredact.RestoreDOCX(prepared, target, m); err != nil {
						return err
					}
					fmt.Fprintf(os.Stderr, "restored %d occurrence(s) -> %s\n", len(m.Occurrences), target)
					return nil
				}
			}
			_, text, err := readInput(input)
			if err != nil {
				return err
			}
			m, err := redactlib.LoadMapping(mappingFile)
			if err != nil {
				return err
			}
			restored, err := redactlib.Restore(text, m, force)
			if err != nil {
				return err
			}
			target := defaultRestoreOut(input)
			if stdout || input == "-" {
				target = "-"
			}
			if err := ensureNotInput(input, target); err != nil {
				return err
			}
			if err := writeOutput(cmd.OutOrStdout(), target, restored); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "restored %d occurrence(s) -> %s\n", len(m.Occurrences), target)
			return nil
		},
	}
	cmd.Flags().BoolVar(&stdout, "stdout", false, "Write the restored text to stdout instead of a file")
	cmd.Flags().StringVar(&mappingFile, "mapping", "", "Mapping file written by redact apply --mapping")
	cmd.Flags().BoolVar(&force, "force", false, "Restore even if the output sha256 or placeholder check fails")
	return cmd
}
