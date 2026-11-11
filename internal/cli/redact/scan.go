package redact

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	clioptions "github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/docredact"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/terminal"
	"github.com/martianzhang/aigc-cli/internal/types"
)

type scanSpan struct {
	Start  int     `json:"start"`
	End    int     `json:"end"`
	Type   string  `json:"type"`
	Text   string  `json:"text"`
	Rule   string  `json:"rule,omitempty"`
	Source string  `json:"source,omitempty"`
	Score  float64 `json:"score,omitempty"`
}

type scanOutput struct {
	Version int        `json:"version"`
	Doc     string     `json:"doc"`
	SHA256  string     `json:"sha256"`
	Offset  string     `json:"offset"`
	Spans   []scanSpan `json:"spans"`
	Review  []scanSpan `json:"needs_review,omitempty"`
	Stats   scanStats  `json:"stats"`
}

type scanStats struct {
	Total  int            `json:"total"`
	Review int            `json:"review,omitempty"`
	ByType map[string]int `json:"by_type"`
}

func newScanCmd() *cobra.Command {
	var flags buildFlags
	var asJSON bool
	var reveal bool
	cmd := &cobra.Command{
		Use:   "scan [file|-]",
		Short: "Detect sensitive spans without modifying the input",
		Example: `  # Detect without changing the file
  aigc-cli redact scan contract.txt

  # Machine-readable spans (rune offsets) for review or apply --spans
  aigc-cli redact scan contract.txt --json > spans.json

  # Add a model for names, addresses and organizations
  aigc-cli --provider ollama --model has4 redact scan contract.txt --engine auto
  aigc-cli --provider local --model-engine tokencls redact scan contract.txt --engine auto

  # Switch to an external rule profile, and toggle individual rules
  aigc-cli redact scan contract.txt --profile strict
  aigc-cli redact scan contract.txt --enable passport,lawyer_no --reveal

  # PDF and image text is extracted/OCR'd automatically
  aigc-cli redact scan judgment.pdf --dpi 300
  aigc-cli redact scan scan.png

  # Read from stdin
  cat contract.txt | aigc-cli redact scan - --json`,
		SilenceUsage: true,
		Args:         cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			loadDeps()
			input := "-"
			if len(args) > 0 {
				input = args[0]
			}
			pipeline, masks, err := buildPipeline(flags)
			if err != nil {
				return err
			}
			if input != "-" {
				prepared, _, cleanup, perr := prepareRedactInput(input)
				if perr != nil {
					return perr
				}
				defer cleanup()
				_, ferr := docredact.Detect(prepared)
				if ferr != nil && docredact.IsDocumentPath(input) {
					return ferr
				}
				if ferr == nil {
					cfg, err := flags.redactConfig()
					if err != nil {
						return err
					}
					res, err := docredact.Scan(cmd.Context(), prepared, pipeline, docredact.Options{
						Masks: masks, DPI: flags.dpi,
						SkipDate: !types.BoolDefault(cfg.Date, true), SkipAmount: !types.BoolDefault(cfg.Amount, true),
						ModelsDir: filepath.Join(clioptions.ConfigDir(), "models"),
					})
					if err != nil {
						return err
					}
					id := filepath.Base(input)
					doc := redactlib.NewDocument(id, res.Text)
					result := redactlib.Result{Spans: res.Spans, Review: res.Review}
					if asJSON {
						out := scanOutput{
							Version: 1, Doc: id, SHA256: doc.SHA256(), Offset: "rune",
							Spans:  toScanSpans(doc, result.Spans),
							Review: toScanSpans(doc, result.Review),
							Stats:  statsFor(result),
						}
						data, err := json.MarshalIndent(out, "", "  ")
						if err != nil {
							return err
						}
						fmt.Fprintln(cmd.OutOrStdout(), string(data))
						return nil
					}
					writeScanTable(cmd, doc, result, masks, reveal)
					return nil
				}
			}
			id, text, err := readInput(input)
			if err != nil {
				return err
			}
			doc := redactlib.NewDocument(id, text)
			result, err := pipeline.Detect(cmd.Context(), doc)
			if err != nil {
				return err
			}
			if asJSON {
				out := scanOutput{
					Version: 1, Doc: id, SHA256: doc.SHA256(), Offset: "rune",
					Spans:  toScanSpans(doc, result.Spans),
					Review: toScanSpans(doc, result.Review),
					Stats:  statsFor(result),
				}
				data, err := json.MarshalIndent(out, "", "  ")
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}
			writeScanTable(cmd, doc, result, masks, reveal)
			return nil
		},
	}
	addDetectFlags(cmd, &flags)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output machine-readable JSON (rune offsets)")
	cmd.Flags().BoolVar(&reveal, "reveal", false, "Show original values in the human-readable table")
	return cmd
}

func toScanSpans(doc *redactlib.Document, spans []redactlib.Span) []scanSpan {
	out := make([]scanSpan, 0, len(spans))
	for _, s := range spans {
		out = append(out, scanSpan{
			Start: doc.RuneOffset(s.Start), End: doc.RuneOffset(s.End),
			Type: s.Type, Text: s.Text, Rule: s.Rule, Source: string(s.Source), Score: s.Score,
		})
	}
	return out
}

func statsFor(r redactlib.Result) scanStats {
	stats := scanStats{Total: len(r.Spans), Review: len(r.Review), ByType: map[string]int{}}
	for _, s := range r.Spans {
		stats.ByType[s.Type]++
	}
	return stats
}

func writeScanTable(cmd *cobra.Command, doc *redactlib.Document, result redactlib.Result, masks *redactlib.MaskResolver, reveal bool) {
	out := cmd.OutOrStdout()
	rows := func(spans []redactlib.Span) [][]string {
		rows := make([][]string, 0, len(spans))
		for _, s := range spans {
			value := s.Text
			if !reveal {
				value = redactlib.MaskValue(s.Text, s.Type, masks.Policy(s.Type))
			}
			rows = append(rows, []string{
				s.Type, value, s.Rule,
				fmt.Sprintf("%d..%d", doc.RuneOffset(s.Start), doc.RuneOffset(s.End)),
			})
		}
		return rows
	}
	headers := []string{"TYPE", "TEXT", "RULE", "POS"}
	_ = terminal.WriteTable(out, headers, rows(result.Spans), terminal.TableOptions{})
	if len(result.Review) > 0 {
		fmt.Fprintln(out, "\nneeds review (not applied automatically):")
		_ = terminal.WriteTable(out, headers, rows(result.Review), terminal.TableOptions{})
	}
	fmt.Fprintf(out, "\n%d span(s), %d need review\n", len(result.Spans), len(result.Review))
}
