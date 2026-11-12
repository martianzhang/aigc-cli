package docredact

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

// PreviewText returns the document text with detected spans replaced and does
// not write an output file. It backs `redact apply --stdout` for DOCX, PDF and
// images so detection results can be reviewed from the command line.
//
// DOCX reuses the exact replacements used for the .docx output. PDF and images
// cannot be represented as text: their extracted text is redacted with
// Options.Style for review, while the file output stays black boxes.
func PreviewText(ctx context.Context, input string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	format, err := Detect(input)
	if err != nil {
		return Result{}, err
	}
	if opts.DPI <= 0 {
		opts.DPI = 300
	}
	switch format {
	case FormatDOCX:
		return previewDOCX(ctx, input, pipeline, opts)
	case FormatPDF:
		res, err := scanPDF(ctx, input, pipeline, opts)
		if err != nil {
			return Result{}, err
		}
		return previewFromScan(res, opts)
	case FormatImage:
		res, err := scanImage(ctx, input, pipeline, opts)
		if err != nil {
			return Result{}, err
		}
		return previewFromScan(res, opts)
	default:
		return Result{}, fmt.Errorf("unsupported document format: %s", filepath.Base(input))
	}
}

func previewDOCX(ctx context.Context, input string, pipeline *redactlib.Pipeline, opts Options) (Result, error) {
	parts, zr, err := extractDOCX(input)
	if err != nil {
		return Result{}, err
	}
	defer zr.Close()
	text, _ := combineDOCXText(parts)
	doc := redactlib.NewDocument(filepath.Base(input), text)
	res, err := pipeline.Detect(ctx, doc)
	if err != nil {
		return Result{}, err
	}
	style := opts.Style
	if style == "" {
		style = "placeholder"
	}
	replacements, mapping, err := docxReplacements(text, res.Spans, style, opts)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Text: applyReplacements(text, replacements), Spans: res.Spans,
		Review: res.Review, Mapping: mapping,
	}, nil
}

// previewFromScan redacts the text extracted by scanPDF/scanImage so the
// preview follows the same text style as plain text input.
func previewFromScan(scan Result, opts Options) (Result, error) {
	doc := redactlib.NewDocument("preview", scan.Text)
	text, mapping, err := redactlib.Apply(doc, scan.Spans, redactlib.ApplyOptions{
		Style: opts.Style, Placeholder: opts.Placeholder, Masks: opts.Masks,
		SkipDate: opts.SkipDate, SkipAmount: opts.SkipAmount, Aliases: opts.Aliases,
	})
	if err != nil {
		return Result{}, err
	}
	scan.Text = text
	scan.Mapping = mapping
	return scan, nil
}

// applyReplacements splices span replacements into text. Replacements are
// produced against the same text document, so ordering alone is enough.
func applyReplacements(text string, replacements []spanReplacement) string {
	sorted := append([]spanReplacement(nil), replacements...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].offset < sorted[j].offset })
	var b strings.Builder
	cursor := 0
	for _, r := range sorted {
		if r.offset < cursor || r.end > len(text) || r.offset >= r.end {
			continue
		}
		b.WriteString(text[cursor:r.offset])
		b.WriteString(r.text)
		cursor = r.end
	}
	b.WriteString(text[cursor:])
	return b.String()
}
