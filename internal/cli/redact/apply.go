package redact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	clioptions "github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/docredact"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func newApplyCmd() *cobra.Command {
	var flags buildFlags
	var stdout bool
	var spansFile string
	var mappingFile string
	var encrypt bool
	var force bool
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
  aigc-cli redact apply contract.txt --mapping contract.map.json --encrypt

  # Word / PDF / image are detected automatically
  aigc-cli redact apply contract.docx --mapping contract.map.json
  aigc-cli redact apply judgment.pdf --dpi 300
  aigc-cli redact apply scan.png

  # Preview Word / PDF / image redaction on stdout without writing a file
  aigc-cli redact apply judgment.pdf --stdout --dpi 300 | grep '<'`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loadDeps()
			input := "-"
			if len(args) > 0 {
				input = args[0]
			}
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
			style := cfg.Style
			if style == "" {
				style = "placeholder"
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
					if spansFile != "" {
						return fmt.Errorf("--spans is only supported for text input")
					}
					if mappingFile != "" && format != docredact.FormatDOCX {
						return fmt.Errorf("--mapping is not supported for %s input", format)
					}
					if mappingFile != "" && style != "placeholder" {
						return fmt.Errorf("--mapping requires style=placeholder for DOCX")
					}
					opts := docredact.Options{
						Style: style, Placeholder: cfg.Placeholder, Masks: masks, Aliases: cfg.Aliases,
						SkipDate: !types.BoolDefault(cfg.Date, true), SkipAmount: !types.BoolDefault(cfg.Amount, true),
						DPI: flags.dpi, ModelsDir: filepath.Join(clioptions.ConfigDir(), "models"),
					}
					if stdout {
						return previewApply(cmd, prepared, pipeline, opts, mappingFile, encrypt)
					}
					target := defaultOut(input, ".redacted")
					if outExt != "" && outExt != filepath.Ext(input) {
						target = strings.TrimSuffix(target, filepath.Ext(target)) + outExt
					}
					if err := ensureNotInput(input, target); err != nil {
						return err
					}
					res, err := docredact.Apply(cmd.Context(), prepared, target, pipeline, opts)
					if err != nil {
						return err
					}
					if mappingFile != "" {
						if res.Mapping == nil {
							return fmt.Errorf("--mapping requires style=placeholder for DOCX")
						}
						if err := redactlib.SaveMapping(mappingFile, res.Mapping, encrypt); err != nil {
							return err
						}
					}
					fmt.Fprintf(os.Stderr, "redacted %d occurrence(s) -> %s\n", len(res.Spans), res.OutputPath)
					if mappingFile == "" && style == "placeholder" && format == docredact.FormatDOCX {
						fmt.Fprintln(os.Stderr, "note: no --mapping written; restore will not be available")
					}
					return nil
				}
			}

			id, text, err := readInput(input)
			if err != nil {
				return err
			}
			doc := redactlib.NewDocument(id, text)
			spans, err := spansToApply(cmd, doc, spansFile, pipeline, force)
			if err != nil {
				return err
			}
			opts := redactlib.ApplyOptions{
				Style: style, Placeholder: cfg.Placeholder,
				SkipDate:   !types.BoolDefault(cfg.Date, true),
				SkipAmount: !types.BoolDefault(cfg.Amount, true),
				Aliases:    cfg.Aliases, Masks: masks,
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
			summaryTo(cmd.ErrOrStderr(), target, output, "redacted %d occurrence(s), %d entities -> %s\n",
				len(mapping.Occurrences), len(mapping.Entities), target)
			if mappingFile == "" && style == "placeholder" {
				fmt.Fprintln(os.Stderr, "note: no --mapping written; restore will not be available")
			}
			return nil
		},
	}
	addDetectFlags(cmd, &flags)
	cmd.Flags().BoolVar(&stdout, "stdout", false, "Write the redacted text to stdout instead of a file (DOCX/PDF/images print a text preview)")
	cmd.Flags().StringVar(&spansFile, "spans", "", "Use reviewed spans from a redact scan --json file instead of detecting again")
	cmd.Flags().StringVar(&mappingFile, "mapping", "", "Write the reversible mapping to this file")
	cmd.Flags().BoolVar(&encrypt, "encrypt", false, "Encrypt the mapping with the aigc-cli master key")
	cmd.Flags().BoolVar(&force, "force", false, "Ignore a sha256 mismatch in --spans")
	return cmd
}

// previewApply prints the redacted text of a document to stdout instead of
// writing a redacted file. DOCX can still export a mapping so the printed text
// stays restorable with `redact restore`.
func previewApply(cmd *cobra.Command, input string, pipeline *redactlib.Pipeline, opts docredact.Options, mappingFile string, encrypt bool) error {
	res, err := docredact.PreviewText(cmd.Context(), input, pipeline, opts)
	if err != nil {
		return err
	}
	if mappingFile != "" {
		if res.Mapping == nil {
			return fmt.Errorf("--mapping requires style=placeholder for DOCX")
		}
		if err := redactlib.SaveMapping(mappingFile, res.Mapping, encrypt); err != nil {
			return err
		}
	}
	if err := writeOutput(cmd.OutOrStdout(), "-", res.Text); err != nil {
		return err
	}
	summaryTo(cmd.ErrOrStderr(), "-", res.Text, "redacted %d occurrence(s) -> stdout (text preview; no file written)\n", len(res.Spans))
	return nil
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
