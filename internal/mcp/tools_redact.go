package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func newRedactTextTool() mcp.Tool {
	return mcp.NewTool("redact_text",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription(`Redact PII in plain text completely offline.

The default engine is built-in rules + checksum validators (ID cards, phones,
bank cards, email, USCC, case numbers, plates, passports, lawyer licences) plus
redact.dictionary exact terms. No API key or network is required.

Returns the redacted text, entity types/placeholders and span positions. Original
values are NOT returned unless include_mapping=true; that mapping is sensitive
because it can restore the document.`),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to redact")),
		mcp.WithString("style", mcp.Enum("placeholder", "mask", "black", "hash"),
			mcp.Description("Replacement style (default: config or placeholder)")),
		mcp.WithBoolean("include_mapping",
			mcp.Description("Include the reversible mapping (contains original values; default false)")),
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
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := req.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError("text is required"), nil
		}
		redactCfg := cfg.Redact
		if redactCfg == nil {
			redactCfg = &types.RedactConfig{}
		}
		rules, err := redactlib.NewRuleEngine(redactCfg)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dict, err := redactlib.NewDictionaryEngine(redactCfg, cfg.ConfigDir)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		masks, err := redactlib.NewMaskResolver(redactCfg)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var engines []redactlib.Engine
		if dict != nil {
			engines = append(engines, dict)
		}
		pipeline := redactlib.NewPipeline(rules, engines...)
		doc := redactlib.NewDocument("mcp", text)
		result, err := pipeline.Detect(context.Background(), doc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		style := req.GetString("style", redactCfg.Style)
		if style == "" {
			style = "placeholder"
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
		if v, ok := req.GetArguments()["include_mapping"].(bool); ok && v {
			resp.Mapping = mapping
		}
		return toolResultJSON(resp)
	}
}

func placeholderForSpan(m *redactlib.Mapping, s redactlib.Span) string {
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
		mcp.WithDescription(`Check text for leftover sensitive values.

Supply either a comma/newline-separated "values" list or a mapping JSON returned
by redact_text (include_mapping=true). Returns the matching positions; no network
or model is used.`),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to check")),
		mcp.WithString("values", mcp.Description("Comma/newline-separated original values")),
		mcp.WithString("mapping", mcp.Description("Mapping JSON from redact_text")),
	)
}

type redactCheckResult struct {
	Count int                  `json:"count"`
	Hits  []redactlib.Residual `json:"hits"`
}

func redactCheckHandler(_ *Config) server.ToolHandlerFunc {
	return func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := req.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError("text is required"), nil
		}
		var mapping *redactlib.Mapping
		if raw := req.GetString("mapping", ""); strings.TrimSpace(raw) != "" {
			var m redactlib.Mapping
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("parse mapping: %v", err)), nil
			}
			mapping = &m
		}
		if mapping == nil {
			values := splitValues(req.GetString("values", ""))
			if len(values) == 0 {
				return mcp.NewToolResultError("values or mapping is required"), nil
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
