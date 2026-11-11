package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func redactRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

func TestRedactTextHandler(t *testing.T) {
	cfg := &Config{Redact: &types.RedactConfig{}}
	res, err := redactTextHandler(cfg)(context.Background(), redactRequest(map[string]any{
		"text": "电话13812345678，身份证110105198503121234。",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}
	var out redactTextResult
	if err := json.Unmarshal([]byte(toolResultText(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.RedactedText, "<PHONE_1>") || strings.Contains(out.RedactedText, "13812345678") {
		t.Fatalf("redacted text = %q", out.RedactedText)
	}
	if len(out.Spans) != 2 || len(out.Entities) != 2 {
		t.Fatalf("spans/entities = %+v / %+v", out.Spans, out.Entities)
	}
	if out.Mapping != nil {
		t.Fatal("mapping must be omitted unless include_mapping=true")
	}
}

func TestRedactTextHandlerIncludeMapping(t *testing.T) {
	cfg := &Config{Redact: &types.RedactConfig{Style: "mask"}}
	res, err := redactTextHandler(cfg)(context.Background(), redactRequest(map[string]any{
		"text": "电话13812345678", "include_mapping": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}
	var out redactTextResult
	if err := json.Unmarshal([]byte(toolResultText(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.RedactedText, "138****5678") {
		t.Fatalf("masked text = %q", out.RedactedText)
	}
	if out.Mapping == nil || len(out.Mapping.Entities) != 1 {
		t.Fatalf("mapping = %+v", out.Mapping)
	}
}

func TestRedactTextHandlerMissingText(t *testing.T) {
	res, err := redactTextHandler(&Config{})(context.Background(), redactRequest(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error for missing text")
	}
}

func TestRedactCheckHandlerValues(t *testing.T) {
	res, err := redactCheckHandler(&Config{})(context.Background(), redactRequest(map[string]any{
		"text":   "漏了13812345678和13812345678",
		"values": "13812345678,不存在",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}
	var out redactCheckResult
	if err := json.Unmarshal([]byte(toolResultText(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || len(out.Hits) != 2 {
		t.Fatalf("check result = %+v", out)
	}
}

func TestRedactCheckHandlerMapping(t *testing.T) {
	mapping := `{"entities":[{"type":"PERSON","value":"张伟","placeholder":"<PERSON_1>"}]}`
	res, err := redactCheckHandler(&Config{})(context.Background(), redactRequest(map[string]any{
		"text": "张伟还在", "mapping": mapping,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}
	var out redactCheckResult
	if err := json.Unmarshal([]byte(toolResultText(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 {
		t.Fatalf("check result = %+v", out)
	}
}

func TestRedactTextHandlerDisableRule(t *testing.T) {
	cfg := &Config{Redact: &types.RedactConfig{}}
	res, err := redactTextHandler(cfg)(context.Background(), redactRequest(map[string]any{
		"text": "电话13812345678", "disable": "phone_mobile,phone_landline,phone_400",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}
	var out redactTextResult
	if err := json.Unmarshal([]byte(toolResultText(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.RedactedText, "<PHONE_1>") || !strings.Contains(out.RedactedText, "13812345678") {
		t.Fatalf("redacted text = %q, want phone kept", out.RedactedText)
	}
}

func TestRedactTextHandlerUnknownRule(t *testing.T) {
	res, err := redactTextHandler(&Config{})(context.Background(), redactRequest(map[string]any{
		"text": "13812345678", "enable": "no_such_rule",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error for unknown rule")
	}
}

func TestRedactTextHandlerEngineAutoWithoutModel(t *testing.T) {
	res, err := redactTextHandler(&Config{Redact: &types.RedactConfig{}})(context.Background(), redactRequest(map[string]any{
		"text": "电话13812345678", "engine": "auto",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}
	var out redactTextResult
	if err := json.Unmarshal([]byte(toolResultText(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.RedactedText, "<PHONE_1>") {
		t.Fatalf("redacted text = %q", out.RedactedText)
	}
}

func TestRedactTextHandlerUnknownEngine(t *testing.T) {
	res, err := redactTextHandler(&Config{})(context.Background(), redactRequest(map[string]any{
		"text": "13812345678", "engine": "bogus",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error for unknown engine")
	}
}

func TestRedactTextHandlerModelOverride(t *testing.T) {
	cfg := &Config{Redact: &types.RedactConfig{Engine: "model"}, ModelsDir: t.TempDir()}
	res, err := redactTextHandler(cfg)(context.Background(), redactRequest(map[string]any{
		"text": "13812345678", "model": "missing-model",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(toolResultText(t, res), "missing-model") {
		t.Fatalf("result = %s, want missing-model error", toolResultText(t, res))
	}
}
