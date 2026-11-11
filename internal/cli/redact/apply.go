package redact

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

func newApplyCmd() *cobra.Command {
	var flags buildFlags
	var stdout bool
	var spansFile string
	var mappingFile string
	var encrypt bool
	var force bool
	var withDate bool
	var withAmount bool
	cmd := &cobra.Command{
		Use:   "apply [file|-]",
		Short: "Detect and replace sensitive spans (scan is optional)",
		Example: `  # Replace detected values and write a reversible mapping
  aigc-cli redact apply contract.txt --mapping contract.map.json

  # Write masked text to stdout (mask is not reversible)
  aigc-cli redact apply contract.txt --stdout --style mask

  # Use an external rule profile
  aigc-cli redact apply contract.txt --profile strict --mapping contract.map.json

  # Apply manually reviewed spans from redact scan --json
  aigc-cli redact scan contract.txt --json > spans.json
  aigc-cli redact apply contract.txt --spans spans.json --mapping contract.map.json

  # Add a model for names, addresses and organizations
  aigc-cli --provider ollama --model has4 redact apply contract.txt --engine auto --mapping contract.map.json

  # Encrypt the mapping (requires the aigc-cli master key)
  aigc-cli redact apply contract.txt --mapping contract.map.json --encrypt`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loadDeps()
			input := "-"
			if len(args) > 0 {
				input = args[0]
			}
			id, text, err := readInput(input)
			if err != nil {
				return err
			}
			doc := redactlib.NewDocument(id, text)

			cfg, err := flags.redactConfig()
			if err != nil {
				return err
			}
			masks, err := redactlib.NewMaskResolver(cfg)
			if err != nil {
				return err
			}
			var pipeline *redactlib.Pipeline
			if spansFile == "" {
				pipeline, masks, err = buildPipeline(flags)
				if err != nil {
					return err
				}
			}
			spans, err := spansToApply(cmd, doc, spansFile, pipeline, force)
			if err != nil {
				return err
			}

			style := cfg.Style
			if style == "" {
				style = "placeholder"
			}
			opts := redactlib.ApplyOptions{
				Style: style, Placeholder: cfg.Placeholder,
				Date: cfg.Date || withDate, Amount: cfg.Amount || withAmount,
				Aliases: cfg.Aliases, Masks: masks,
			}
			output, mapping, err := redactlib.Apply(doc, spans, opts)
			if err != nil {
				return err
			}
			target := defaultOut(input, ".redacted")
			if stdout || input == "-" {
				target = "-"
			}
			if err := ensureNotInput(input, target); err != nil {
				return err
			}
			if err := writeOutput(cmd.OutOrStdout(), target, output); err != nil {
				return err
			}
			if mappingFile != "" {
				if err := redactlib.SaveMapping(mappingFile, mapping, encrypt); err != nil {
					return err
				}
			}
			fmt.Fprintf(os.Stderr, "redacted %d occurrence(s), %d entities -> %s\n",
				len(mapping.Occurrences), len(mapping.Entities), target)
			if mappingFile == "" && style == "placeholder" {
				fmt.Fprintln(os.Stderr, "note: no --mapping written; restore will not be available")
			}
			return nil
		},
	}
	addDetectFlags(cmd, &flags)
	cmd.Flags().BoolVar(&stdout, "stdout", false, "Write the redacted text to stdout instead of a file")
	cmd.Flags().StringVar(&spansFile, "spans", "", "Use reviewed spans from a redact scan --json file instead of detecting again")
	cmd.Flags().StringVar(&mappingFile, "mapping", "", "Write the reversible mapping to this file")
	cmd.Flags().BoolVar(&encrypt, "encrypt", false, "Encrypt the mapping with the aigc-cli master key")
	cmd.Flags().BoolVar(&force, "force", false, "Ignore a sha256 mismatch in --spans")
	cmd.Flags().BoolVar(&withDate, "with-date", false, "Redact DATE spans (off by default)")
	cmd.Flags().BoolVar(&withAmount, "with-amount", false, "Redact AMOUNT spans (off by default)")
	return cmd
}

func spansToApply(cmd *cobra.Command, doc *redactlib.Document, path string, pipeline *redactlib.Pipeline, force bool) ([]redactlib.Span, error) {
	if path == "" {
		result, err := pipeline.Detect(cmd.Context(), doc)
		if err != nil {
			return nil, err
		}
		return result.Spans, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spans: %w", err)
	}
	var parsed scanOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse spans: %w", err)
	}
	if parsed.SHA256 != "" && parsed.SHA256 != doc.SHA256() && !force {
		return nil, fmt.Errorf("spans were produced for a different document (sha256 mismatch); use --force to override")
	}
	spans := make([]redactlib.Span, 0, len(parsed.Spans))
	for _, s := range parsed.Spans {
		start, end := s.Start, s.End
		if parsed.Offset != "byte" {
			start = doc.ByteOffset(s.Start)
			end = doc.ByteOffset(s.End)
		}
		if start < 0 || end > len(doc.Text) || start >= end {
			continue
		}
		spans = append(spans, redactlib.Span{
			Start: start, End: end, Type: s.Type, Text: doc.Text[start:end],
			Rule: s.Rule, Source: redactlib.SourceManual, Score: s.Score, Priority: 1000,
		})
	}
	return spans, nil
}
