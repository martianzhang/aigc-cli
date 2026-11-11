package mcp

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/martianzhang/aigc-cli/internal/docredact"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func writeRedactDOCX(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(w, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>%s</w:t></w:r></w:p></w:body></w:document>`, html.EscapeString(text))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRedactScanHandlerText(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(input, []byte("电话13812345678。"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := redactScanHandler(&Config{Redact: &types.RedactConfig{}})(context.Background(), redactRequest(map[string]any{
		"file_path": input, "include_text": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}
	var out redactFileResult
	if err := json.Unmarshal([]byte(toolResultText(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if out.Format != "text" || len(out.Spans) != 1 || out.Spans[0].Type != "PHONE" {
		t.Fatalf("result = %+v", out)
	}
	if out.Text != "电话13812345678。" {
		t.Fatalf("text = %q", out.Text)
	}
}

func TestRedactApplyHandlerText(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.txt")
	output := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(input, []byte("电话13812345678。"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := redactApplyHandler(&Config{Redact: &types.RedactConfig{}})(context.Background(), redactRequest(map[string]any{
		"file_path": input, "output_path": output, "style": "mask",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("handler error: %s", toolResultText(t, res))
	}
	var out redactFileResult
	if err := json.Unmarshal([]byte(toolResultText(t, res)), &out); err != nil {
		t.Fatal(err)
	}
	if out.OutputPath != output {
		t.Fatalf("output_path = %q, want %q", out.OutputPath, output)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if nd := string(data); !strings.Contains(nd, "138****5678") || strings.Contains(nd, "13812345678") {
		t.Fatalf("output = %q", nd)
	}
}

func TestRedactApplyRestoreDOCX(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.docx")
	redacted := filepath.Join(dir, "in.redacted.docx")
	restored := filepath.Join(dir, "in.restored.docx")
	mapping := filepath.Join(dir, "map.json")
	writeRedactDOCX(t, input, "原告：张伟，电话13812345678。")

	res, err := redactApplyHandler(&Config{Redact: &types.RedactConfig{}})(context.Background(), redactRequest(map[string]any{
		"file_path": input, "output_path": redacted, "style": "placeholder",
		"mapping_path": mapping, "include_mapping": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("apply error: %s", toolResultText(t, res))
	}
	var applied redactFileResult
	if err := json.Unmarshal([]byte(toolResultText(t, res)), &applied); err != nil {
		t.Fatal(err)
	}
	if applied.Mapping == nil || len(applied.Spans) != 2 {
		t.Fatalf("apply result = %+v", applied)
	}
	if _, err := os.Stat(mapping); err != nil {
		t.Fatalf("mapping not written: %v", err)
	}

	res, err = redactRestoreHandler(&Config{})(context.Background(), redactRequest(map[string]any{
		"file_path": redacted, "output_path": restored, "mapping_path": mapping,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("restore error: %s", toolResultText(t, res))
	}
	text, err := docredact.ExtractText(restored)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "张伟") || !strings.Contains(text, "13812345678") {
		t.Fatalf("restored text = %q", text)
	}
}

func TestRedactRestoreRejectsNonDOCX(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(input, []byte("text"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := redactRestoreHandler(&Config{})(context.Background(), redactRequest(map[string]any{
		"file_path": input, "mapping": `{"entities":[]}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected error for non-DOCX restore")
	}
}

func TestRedactCheckHandlerFilePath(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(input, []byte("残留13812345678"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := redactCheckHandler(&Config{})(context.Background(), redactRequest(map[string]any{
		"file_path": input, "values": "13812345678",
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

func TestRedactFileToolSchemas(t *testing.T) {
	cases := []struct {
		tool     mcp.Tool
		required string
		props    []string
	}{
		{
			tool: newRedactScanTool(), required: "file_path",
			props: []string{"dpi", "include_text", "engine", "enable", "disable", "model", "provider", "allow_remote"},
		},
		{
			tool: newRedactApplyTool(), required: "file_path",
			props: []string{"output_path", "style", "dpi", "include_mapping", "mapping_path", "encrypt_mapping", "engine", "enable", "disable", "model", "provider", "allow_remote"},
		},
		{
			tool: newRedactRestoreTool(), required: "file_path",
			props: []string{"output_path", "mapping", "mapping_path"},
		},
	}
	for _, c := range cases {
		if len(c.tool.InputSchema.Required) != 1 || c.tool.InputSchema.Required[0] != c.required {
			t.Errorf("%s required = %v, want [%s]", c.tool.Name, c.tool.InputSchema.Required, c.required)
		}
		for _, prop := range c.props {
			if _, ok := c.tool.InputSchema.Properties[prop]; !ok {
				t.Errorf("%s missing property %q", c.tool.Name, prop)
			}
		}
	}
}

func TestRedactFormatLegacyDOC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.doc")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := redactFormat(path); err == nil || !strings.Contains(err.Error(), "legacy .doc") {
		t.Fatalf("error = %v, want legacy .doc guidance", err)
	}
}
