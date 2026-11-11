package mcp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/docredact"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const maxRedactFileBytes = 20 << 20

// redactFileResult is shared by redact_scan/apply/restore. Text is only
// returned on scan when include_text=true; apply writes it to a file.
type redactFileResult struct {
	Format     string              `json:"format"`
	OutputPath string              `json:"output_path,omitempty"`
	Restored   bool                `json:"restored,omitempty"`
	Text       string              `json:"text,omitempty"`
	Spans      []redactSpanSummary `json:"spans,omitempty"`
	Review     []redactSpanSummary `json:"review,omitempty"`
	Mapping    *redactlib.Mapping  `json:"mapping,omitempty"`
}

func newRedactScanTool() mcp.Tool {
	opts := []mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription(`Scan a text, DOCX, PDF or image file for sensitive spans without writing anything.

Uses the same detection pipeline as the CLI: rules + checksum validators +
redact.dictionary by default, optionally a configured model with
engine="auto" or "model". A non-loopback model provider also requires
allow_remote=true.

Returns format, span positions (rune offsets) and review spans. Set
include_text=true to also return the extracted text (sensitive).`),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Path to a text, DOCX, PDF or image file")),
		mcp.WithNumber("dpi", mcp.Description("PDF rendering DPI (default 300)")),
		mcp.WithBoolean("include_text", mcp.Description("Also return the extracted text (sensitive; default false)")),
	}
	opts = append(opts, redactEngineArgs()...)
	return mcp.NewTool("redact_scan", opts...)
}

func newRedactApplyTool() mcp.Tool {
	opts := []mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(`Redact a text, DOCX, PDF or image file and write the result.

DOCX and text use the configured style (placeholder/mask/black/hash). PDF and
images always use black boxes. PDF rendering requires mutool; PDF/image OCR
requires "aigc-cli ocr init".

Output defaults to <name>.redacted<ext> next to the input (or output_path).
For DOCX/text with style=placeholder, set include_mapping=true or mapping_path
to obtain the reversible mapping. PDF/image do not support mapping.`),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Path to a text, DOCX, PDF or image file")),
		mcp.WithString("output_path", mcp.Description("Output file (or an existing directory); default <name>.redacted<ext>")),
		mcp.WithString("style", mcp.Enum("placeholder", "mask", "black", "hash"),
			mcp.Description("Replacement style for text/DOCX (PDF/image always use black)")),
		mcp.WithNumber("dpi", mcp.Description("PDF rendering DPI (default 300)")),
		mcp.WithBoolean("include_mapping", mcp.Description("Return the reversible mapping (contains originals)")),
		mcp.WithString("mapping_path", mcp.Description("Write the reversible mapping to this file")),
		mcp.WithBoolean("encrypt_mapping", mcp.Description("Encrypt the mapping with the aigc-cli master key")),
	}
	opts = append(opts, redactEngineArgs()...)
	return mcp.NewTool("redact_apply", opts...)
}

func newRedactRestoreTool() mcp.Tool {
	return mcp.NewTool("redact_restore",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription(`Restore a redacted DOCX from a placeholder mapping.

Requires a mapping returned by redact_apply (inline "mapping" JSON or
"mapping_path"). Output defaults to <name>.restored.docx next to the input.
Only DOCX is supported; PDF and images are not reversible.`),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Path to the redacted DOCX")),
		mcp.WithString("output_path", mcp.Description("Output DOCX (or an existing directory); default <name>.restored.docx")),
		mcp.WithString("mapping", mcp.Description("Mapping JSON from redact_apply")),
		mcp.WithString("mapping_path", mcp.Description("Path to a mapping file written by redact_apply")),
	)
}

// redactEngineArgs returns the shared per-call engine/provider schema.
func redactEngineArgs() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("engine", mcp.Enum("rules", "model", "auto"),
			mcp.Description("Detection engine: rules (default), model, or auto (rules + model)")),
		mcp.WithString("enable", mcp.Description("Comma-separated rule names to enable for this call")),
		mcp.WithString("disable", mcp.Description("Comma-separated rule names to disable for this call")),
		mcp.WithString("model", mcp.Description("Override the model name/pack for this call (e.g. has4, multilingual-ner)")),
		mcp.WithString("provider", mcp.Description(providerArgDesc)),
		mcp.WithBoolean("allow_remote",
			mcp.Description("Allow sending text to a non-loopback model provider (default false)")),
	}
}

func redactScanHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		input, format, _, cleanup, err := redactInput(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer cleanup()
		pipeline, masks, redactCfg, err := cfg.redactPipeline(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var res docredact.Result
		if format == "text" {
			res, err = scanRedactText(ctx, input, pipeline)
		} else {
			opts := cfg.redactFileOptions(redactCfg, masks, req.GetInt("dpi", 0), "")
			res, err = docredact.Scan(ctx, input, pipeline, opts)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		resp := redactFileResult{
			Format: format,
			Spans:  summarizeRedactSpans(res.Text, res.Spans, nil),
			Review: summarizeRedactSpans(res.Text, res.Review, nil),
		}
		if req.GetBool("include_text", false) {
			resp.Text = res.Text
		}
		return toolResultJSON(resp)
	}
}

func redactApplyHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		input, format, outExt, cleanup, err := redactInput(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer cleanup()
		pipeline, masks, redactCfg, err := cfg.redactPipeline(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		style := req.GetString("style", redactCfg.Style)
		if style == "" {
			style = "placeholder"
		}
		original, perr := resolveRedactPath(req.GetString("file_path", ""))
		if perr != nil {
			return mcp.NewToolResultError(perr.Error()), nil
		}
		output := redactOutputPath(original, req.GetString("output_path", ""), ".redacted")
		if outExt != "" && outExt != filepath.Ext(original) {
			output = strings.TrimSuffix(output, filepath.Ext(output)) + outExt
		}
		if err := ensureRedactNotInput(original, output); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		includeMapping := req.GetBool("include_mapping", false)
		mappingPath := strings.TrimSpace(req.GetString("mapping_path", ""))
		switch format {
		case "pdf", "image":
			if includeMapping || mappingPath != "" {
				return mcp.NewToolResultError(fmt.Sprintf("mapping is not supported for %s input", format)), nil
			}
		case "docx":
			if (includeMapping || mappingPath != "") && style != "placeholder" {
				return mcp.NewToolResultError("mapping requires style=placeholder for DOCX"), nil
			}
		}

		var res docredact.Result
		if format == "text" {
			res, err = applyRedactText(ctx, input, output, pipeline, masks, redactCfg, style)
		} else {
			opts := cfg.redactFileOptions(redactCfg, masks, req.GetInt("dpi", 0), style)
			res, err = docredact.Apply(ctx, input, output, pipeline, opts)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if mappingPath != "" {
			if res.Mapping == nil {
				return mcp.NewToolResultError("mapping is not available for this input"), nil
			}
			if err := os.MkdirAll(filepath.Dir(mappingPath), 0o755); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if err := redactlib.SaveMapping(mappingPath, res.Mapping, req.GetBool("encrypt_mapping", false)); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
		resp := redactFileResult{
			Format:     format,
			OutputPath: res.OutputPath,
			Spans:      summarizeRedactSpans(res.Text, res.Spans, res.Mapping),
			Review:     summarizeRedactSpans(res.Text, res.Review, res.Mapping),
		}
		if includeMapping {
			resp.Mapping = res.Mapping
		}
		return toolResultJSON(resp)
	}
}

func redactRestoreHandler(_ *Config) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		input, format, outExt, cleanup, err := redactInput(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		defer cleanup()
		if format != "docx" {
			return mcp.NewToolResultError(fmt.Sprintf("restore is not supported for %s input", format)), nil
		}
		mapping, err := redactMappingFromRequest(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if mapping == nil {
			return mcp.NewToolResultError("mapping or mapping_path is required"), nil
		}
		original, perr := resolveRedactPath(req.GetString("file_path", ""))
		if perr != nil {
			return mcp.NewToolResultError(perr.Error()), nil
		}
		output := redactRestoreOutput(original, req.GetString("output_path", ""))
		if outExt != "" && outExt != filepath.Ext(original) {
			output = strings.TrimSuffix(output, filepath.Ext(output)) + outExt
		}
		if err := ensureRedactNotInput(original, output); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := docredact.RestoreDOCX(input, output, mapping); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return toolResultJSON(redactFileResult{Format: "docx", OutputPath: output, Restored: true})
	}
}

func redactInput(req mcp.CallToolRequest) (string, string, string, func(), error) {
	noop := func() {}
	raw, err := req.RequireString("file_path")
	if err != nil {
		return "", "", "", noop, fmt.Errorf("file_path is required")
	}
	input, err := resolveRedactPath(raw)
	if err != nil {
		return "", "", "", noop, err
	}
	prepared, outExt, cleanup, err := docredact.PrepareInput(input)
	if err != nil {
		return "", "", "", noop, err
	}
	format, err := redactFormat(prepared)
	if err != nil {
		cleanup()
		return "", "", "", noop, err
	}
	return prepared, format, outExt, cleanup, nil
}

// redactFormat identifies document formats first and falls back to common
// plain-text extensions.
func redactFormat(path string) (string, error) {
	format, err := docredact.Detect(path)
	if err == nil {
		return format.String(), nil
	}
	// Known document containers must surface the precise reason (legacy .doc,
	// unsupported spreadsheet, ...) instead of falling back to text.
	if docredact.IsDocumentPath(path) {
		return "", err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".markdown", ".csv", ".json", ".yaml", ".yml",
		".xml", ".html", ".htm", ".log":
		return "text", nil
	}
	return "", fmt.Errorf("unsupported document format: %s", filepath.Base(path))
}

func readRedactText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRedactFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if len(data) > maxRedactFileBytes {
		return "", fmt.Errorf("input exceeds %d MB limit", maxRedactFileBytes>>20)
	}
	return string(data), nil
}

func scanRedactText(ctx context.Context, input string, pipeline *redactlib.Pipeline) (docredact.Result, error) {
	text, err := readRedactText(input)
	if err != nil {
		return docredact.Result{}, err
	}
	doc := redactlib.NewDocument(filepath.Base(input), text)
	res, err := pipeline.Detect(ctx, doc)
	if err != nil {
		return docredact.Result{}, err
	}
	return docredact.Result{Text: text, Spans: res.Spans, Review: res.Review}, nil
}

func applyRedactText(ctx context.Context, input, output string, pipeline *redactlib.Pipeline, masks *redactlib.MaskResolver, redactCfg *types.RedactConfig, style string) (docredact.Result, error) {
	text, err := readRedactText(input)
	if err != nil {
		return docredact.Result{}, err
	}
	doc := redactlib.NewDocument(filepath.Base(input), text)
	res, err := pipeline.Detect(ctx, doc)
	if err != nil {
		return docredact.Result{}, err
	}
	out, mapping, err := redactlib.Apply(doc, res.Spans, redactlib.ApplyOptions{
		Style: style, Placeholder: redactCfg.Placeholder,
		SkipDate:   !types.BoolDefault(redactCfg.Date, true),
		SkipAmount: !types.BoolDefault(redactCfg.Amount, true),
		Aliases:    redactCfg.Aliases, Masks: masks,
	})
	if err != nil {
		return docredact.Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return docredact.Result{}, err
	}
	if err := os.WriteFile(output, []byte(out), 0o600); err != nil {
		return docredact.Result{}, fmt.Errorf("write %s: %w", filepath.Base(output), err)
	}
	return docredact.Result{OutputPath: output, Text: text, Spans: res.Spans, Review: res.Review, Mapping: mapping}, nil
}

func (cfg *Config) redactFileOptions(redactCfg *types.RedactConfig, masks *redactlib.MaskResolver, dpi int, style string) docredact.Options {
	modelsDir := cfg.ModelsDir
	if modelsDir == "" {
		modelsDir = filepath.Join(cfg.ConfigDir, "models")
	}
	return docredact.Options{
		Style: style, Placeholder: redactCfg.Placeholder, Masks: masks, Aliases: redactCfg.Aliases,
		SkipDate:   !types.BoolDefault(redactCfg.Date, true),
		SkipAmount: !types.BoolDefault(redactCfg.Amount, true),
		DPI:        dpi, ModelsDir: modelsDir,
	}
}

func summarizeRedactSpans(text string, spans []redactlib.Span, mapping *redactlib.Mapping) []redactSpanSummary {
	if len(spans) == 0 {
		return nil
	}
	doc := redactlib.NewDocument("result", text)
	out := make([]redactSpanSummary, 0, len(spans))
	for _, s := range spans {
		out = append(out, redactSpanSummary{
			Start: doc.RuneOffset(s.Start), End: doc.RuneOffset(s.End),
			Type: s.Type, Placeholder: placeholderForSpan(mapping, s),
		})
	}
	return out
}

func redactOutputPath(input, raw, suffix string) string {
	if strings.TrimSpace(raw) == "" {
		return redactDefaultOutput(input, suffix)
	}
	return normalizeRedactOutput(input, raw, suffix)
}

func redactRestoreOutput(input, raw string) string {
	if strings.TrimSpace(raw) != "" {
		return normalizeRedactOutput(input, raw, ".restored")
	}
	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	stem = strings.TrimSuffix(stem, ".redacted")
	return filepath.Join(filepath.Dir(input), stem+".restored"+ext)
}

func redactDefaultOutput(input, suffix string) string {
	ext := filepath.Ext(input)
	stem := strings.TrimSuffix(filepath.Base(input), ext)
	return filepath.Join(filepath.Dir(input), stem+suffix+ext)
}

func normalizeRedactOutput(input, raw, suffix string) string {
	raw = strings.TrimSpace(raw)
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			raw = filepath.Join(home, strings.TrimPrefix(raw, "~/"))
		}
	}
	if st, err := os.Stat(raw); err == nil && st.IsDir() {
		return filepath.Join(raw, filepath.Base(redactDefaultOutput(input, suffix)))
	}
	if abs, err := filepath.Abs(raw); err == nil {
		return abs
	}
	return raw
}

func ensureRedactNotInput(input, output string) error {
	in, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	out, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if filepath.Clean(in) == filepath.Clean(out) {
		return fmt.Errorf("refusing to overwrite input %s (choose another output_path)", input)
	}
	return nil
}
