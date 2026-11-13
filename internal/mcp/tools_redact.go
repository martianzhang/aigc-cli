package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/docredact"
	"github.com/martianzhang/aigc-cli/internal/provider"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const redactCmdName = "redact"

func newRedactTextTool() mcp.Tool {
	return mcp.NewTool("redact_text",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription(`Redact PII in plain text.

The default engine is built-in rules + checksum validators (ID cards, phones,
bank cards, email, USCC, case numbers, plates, passports, lawyer licences) plus
redact.dictionary exact terms. No API key or network is required.

Set engine="auto" or "model" to add the configured model (local tokencls or a
chat provider). A non-loopback provider also requires allow_remote=true.

Returns the redacted text, entity types/placeholders and span positions. Original
values are NOT returned unless include_mapping=true; that mapping is sensitive
because it can restore the document.`),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to redact")),
		mcp.WithString("style", mcp.Enum("placeholder", "mask", "black", "hash"),
			mcp.Description("Replacement style (default: config or placeholder)")),
		mcp.WithBoolean("include_mapping",
			mcp.Description("Include the reversible mapping (contains original values; default false)")),
		mcp.WithString("engine", mcp.Enum("rules", "model", "auto"),
			mcp.Description("Detection engine: rules (default), model, or auto (rules + model)")),
		mcp.WithString("enable", mcp.Description("Comma-separated rule names to enable for this call")),
		mcp.WithString("disable", mcp.Description("Comma-separated rule names to disable for this call")),
		mcp.WithString("model", mcp.Description("Override the model name/pack for this call (e.g. has4, multilingual-ner)")),
		mcp.WithString("provider", mcp.Description(providerArgDesc)),
		mcp.WithBoolean("allow_remote",
			mcp.Description("Allow sending text to a non-loopback model provider (default false)")),
	)
}

type redactSpanSummary struct {
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Type        string `json:"type"`
	Placeholder string `json:"placeholder,omitempty"`
}

type redactEntitySummary struct {
	Type        string `json:"type"`
	Placeholder string `json:"placeholder"`
	Count       int    `json:"count"`
}

type redactTextResult struct {
	RedactedText string                `json:"redacted_text"`
	Style        string                `json:"style"`
	Spans        []redactSpanSummary   `json:"spans"`
	Entities     []redactEntitySummary `json:"entities"`
	Mapping      *redactlib.Mapping    `json:"mapping,omitempty"`
}

func redactTextHandler(cfg *Config) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := req.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError("text is required"), nil
		}
		pipeline, masks, redactCfg, err := cfg.redactPipeline(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		style := req.GetString("style", redactCfg.Style)
		if style == "" {
			style = "placeholder"
		}
		doc := redactlib.NewDocument("mcp", text)
		result, err := pipeline.Detect(ctx, doc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out, mapping, err := redactlib.Apply(doc, result.Spans, redactlib.ApplyOptions{
			Style: style, Placeholder: redactCfg.Placeholder,
			SkipDate:   !types.BoolDefault(redactCfg.Date, true),
			SkipAmount: !types.BoolDefault(redactCfg.Amount, true),
			Aliases:    redactCfg.Aliases, Masks: masks,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		resp := redactTextResult{RedactedText: out, Style: style}
		for _, s := range result.Spans {
			resp.Spans = append(resp.Spans, redactSpanSummary{
				Start: doc.RuneOffset(s.Start), End: doc.RuneOffset(s.End),
				Type: s.Type, Placeholder: placeholderForSpan(mapping, s),
			})
		}
		for _, e := range mapping.Entities {
			resp.Entities = append(resp.Entities, redactEntitySummary{
				Type: e.Type, Placeholder: e.Placeholder, Count: e.Count,
			})
		}
		if req.GetBool("include_mapping", false) {
			resp.Mapping = mapping
		}
		return toolResultJSON(resp)
	}
}

// redactPipeline builds the same rules/model pipeline as the CLI. Per-call
// args may override the engine, toggle rules, and choose a named provider.
func (cfg *Config) redactPipeline(req mcp.CallToolRequest) (*redactlib.Pipeline, *redactlib.MaskResolver, *types.RedactConfig, error) {
	redactCfg := cloneRedactConfig(cfg.Redact)
	modelOverride := strings.TrimSpace(req.GetString("model", ""))
	if modelOverride != "" {
		redactCfg.Model = modelOverride
		if redactCfg.Tokencls == nil {
			redactCfg.Tokencls = &types.RedactTokenclsConfig{}
		} else {
			copy := *redactCfg.Tokencls
			redactCfg.Tokencls = &copy
		}
		redactCfg.Tokencls.Model = modelOverride
	}
	if err := applyRuleToggles(redactCfg, req.GetString("enable", ""), req.GetString("disable", "")); err != nil {
		return nil, nil, nil, err
	}
	engine := strings.ToLower(strings.TrimSpace(req.GetString("engine", redactCfg.Engine)))
	if engine == "" {
		engine = "rules"
	}
	switch engine {
	case "rules", "model", "auto":
	default:
		return nil, nil, nil, fmt.Errorf("unknown engine %q (want rules, model or auto)", engine)
	}

	p, msg := cfg.resolveProviderRef(redactCmdName, req.GetString("provider", ""))
	if msg != "" {
		return nil, nil, nil, fmt.Errorf("%s", msg)
	}

	var ruleLayer *redactlib.RuleEngine
	if engine != "model" {
		rules, err := redactlib.NewRuleEngine(redactCfg)
		if err != nil {
			return nil, nil, nil, err
		}
		ruleLayer = rules
	}
	var engines []redactlib.Engine
	dict, err := redactlib.NewDictionaryEngine(redactCfg, cfg.ConfigDir)
	if err != nil {
		return nil, nil, nil, err
	}
	if dict != nil {
		engines = append(engines, dict)
	}

	if engine == "model" || engine == "auto" {
		useTokencls, err := redactUseTokencls(redactCfg, p)
		if err != nil {
			return nil, nil, nil, err
		}
		switch {
		case useTokencls:
			eng, err := cfg.tokenclsEngine(redactCfg, p)
			switch {
			case err == nil:
				engines = append(engines, eng)
			case engine == "model":
				return nil, nil, nil, err
			}
		case p == nil || p.BaseURL == "":
			if engine == "model" {
				return nil, nil, nil, fmt.Errorf("engine=model requires a provider; set defaults.redact.provider or pass provider")
			}
		default:
			if !req.GetBool("allow_remote", false) && !provider.IsLocalEndpoint(p.BaseURL) {
				return nil, nil, nil, fmt.Errorf("model endpoint %s is not loopback; pass allow_remote=true only if sending original text is acceptable", p.BaseURL)
			}
			model := modelOverride
			if model == "" {
				model = p.Model
			}
			if model == "" {
				model = redactCfg.Model
			}
			if model == "" {
				model = "has4"
			}
			eng, err := redactlib.NewChatner(client.NewFromProvider(p), model, redactCfg.Chatner)
			if err != nil {
				return nil, nil, nil, err
			}
			engines = append(engines, eng)
		}
	}

	masks, err := redactlib.NewMaskResolver(redactCfg)
	if err != nil {
		return nil, nil, nil, err
	}
	return redactlib.NewPipeline(ruleLayer, engines...), masks, redactCfg, nil
}

func redactUseTokencls(redactCfg *types.RedactConfig, p *provider.EffectiveProvider) (bool, error) {
	mode := strings.ToLower(strings.TrimSpace(redactCfg.ModelEngine))
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "tokencls":
		return true, nil
	case "chatner":
		return false, nil
	case "auto":
		switch {
		case p != nil && p.Type == types.ProviderLocal:
			return true, nil
		case p != nil && p.BaseURL != "":
			return false, nil
		default:
			return redactCfg.Tokencls != nil, nil
		}
	default:
		return false, fmt.Errorf("unknown model engine %q (want auto, chatner or tokencls)", mode)
	}
}

func (cfg *Config) tokenclsEngine(redactCfg *types.RedactConfig, p *provider.EffectiveProvider) (redactlib.Engine, error) {
	tcCfg := &types.RedactTokenclsConfig{}
	if redactCfg.Tokencls != nil {
		*tcCfg = *redactCfg.Tokencls
	}
	modelsDir := cfg.ModelsDir
	if p != nil {
		if p.ModelsDir != "" {
			modelsDir = p.ModelsDir
		}
		if tcCfg.Model == "" {
			tcCfg.Model = p.Model
		}
	}
	if modelsDir == "" {
		modelsDir = filepath.Join(cfg.ConfigDir, "models")
	}
	return redactlib.NewTokenClassifier(tcCfg, modelsDir)
}

func cloneRedactConfig(in *types.RedactConfig) *types.RedactConfig {
	out := &types.RedactConfig{}
	if in != nil {
		*out = *in
	}
	if in != nil && in.Rules != nil {
		out.Rules = make(map[string]bool, len(in.Rules))
		for k, v := range in.Rules {
			out.Rules[k] = v
		}
	}
	return out
}

func applyRuleToggles(redactCfg *types.RedactConfig, enable, disable string) error {
	known := make(map[string]bool)
	for _, name := range redactlib.BuiltinRuleNames() {
		known[name] = true
	}
	for _, custom := range redactCfg.Custom {
		known[custom.Name] = true
	}
	if redactCfg.Rules == nil {
		redactCfg.Rules = make(map[string]bool)
	}
	for _, name := range splitCSV(enable) {
		if !known[name] {
			return fmt.Errorf("unknown rule %q in enable", name)
		}
		redactCfg.Rules[name] = true
	}
	for _, name := range splitCSV(disable) {
		if !known[name] {
			return fmt.Errorf("unknown rule %q in disable", name)
		}
		redactCfg.Rules[name] = false
	}
	return nil
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func placeholderForSpan(m *redactlib.Mapping, s redactlib.Span) string {
	if m == nil {
		return ""
	}
	for _, e := range m.Entities {
		if e.Type != s.Type {
			continue
		}
		if e.Value == s.Text {
			return e.Placeholder
		}
		for _, alias := range e.Aliases {
			if alias == s.Text {
				return e.Placeholder
			}
		}
	}
	return ""
}

func toolResultJSON(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("encode result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func newRedactCheckTool() mcp.Tool {
	return mcp.NewTool("redact_check",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription(`Check text or a DOCX file for leftover sensitive values.

Supply "text" or "file_path" (DOCX or plain text). Supply either a
comma/newline-separated "values" list, an inline mapping JSON, or a
mapping_path written by redact_apply. Returns the matching positions; no network
or model is used.`),
		mcp.WithString("text", mcp.Description("Text to check")),
		mcp.WithString("file_path", mcp.Description("Path to a DOCX or plain-text file to check")),
		mcp.WithString("values", mcp.Description("Comma/newline-separated original values")),
		mcp.WithString("mapping", mcp.Description("Mapping JSON from redact_text/redact_apply")),
		mcp.WithString("mapping_path", mcp.Description("Path to a mapping file written by redact_apply")),
	)
}

type redactCheckResult struct {
	Count int                  `json:"count"`
	Hits  []redactlib.Residual `json:"hits"`
}

func redactCheckHandler(_ *Config) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text := req.GetString("text", "")
		filePath := strings.TrimSpace(req.GetString("file_path", ""))
		if filePath != "" {
			if text != "" {
				return mcp.NewToolResultError("pass text or file_path, not both"), nil
			}
			path, err := resolveRedactPath(filePath)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			path, _, cleanup, err := docredact.PrepareInput(path)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			defer cleanup()
			format, err := redactFormat(path)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			switch format {
			case "text":
				text, err = readRedactText(path)
			case "docx":
				text, err = docredact.ExtractText(path)
			default:
				return mcp.NewToolResultError(fmt.Sprintf("check is not supported for %s input", format)), nil
			}
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
		}
		if text == "" {
			return mcp.NewToolResultError("text or file_path is required"), nil
		}

		mapping, err := redactMappingFromRequest(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if mapping == nil {
			values := splitValues(req.GetString("values", ""))
			if len(values) == 0 {
				return mcp.NewToolResultError("values, mapping or mapping_path is required"), nil
			}
			mapping = &redactlib.Mapping{Entities: make([]redactlib.Entity, 0, len(values))}
			for _, v := range values {
				mapping.Entities = append(mapping.Entities, redactlib.Entity{Type: "SENSITIVE", Value: v})
			}
		}
		hits := redactlib.Check(text, mapping)
		return toolResultJSON(redactCheckResult{Count: len(hits), Hits: hits})
	}
}

// redactMappingFromRequest loads a mapping from mapping_path (supporting
// encrypted files) or an inline mapping JSON string.
func redactMappingFromRequest(req mcp.CallToolRequest) (*redactlib.Mapping, error) {
	if path := strings.TrimSpace(req.GetString("mapping_path", "")); path != "" {
		m, err := redactlib.LoadMapping(path)
		if err != nil {
			return nil, err
		}
		return m, nil
	}
	raw := strings.TrimSpace(req.GetString("mapping", ""))
	if raw == "" {
		return nil, nil
	}
	var m redactlib.Mapping
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("parse mapping: %w", err)
	}
	return &m, nil
}

func splitValues(raw string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// resolveRedactPath expands a leading ~ and makes the path absolute.
func resolveRedactPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("file_path is required\nHint: pass an absolute file_path in the tool arguments")
	}
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		raw = filepath.Join(home, strings.TrimPrefix(raw, "~/"))
	}
	path, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("file not found: %s", path)
	}
	return path, nil
}
