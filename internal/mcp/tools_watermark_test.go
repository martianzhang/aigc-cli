package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func cropWatermarkRequest(input, output string) mcp.CallToolRequest {
	args := map[string]any{"file_path": input, "target": "auto"}
	if output != "" {
		args["output_path"] = output
	}
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "crop_watermark", Arguments: args},
	}
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		return ""
	}
	text, _ := res.Content[0].(mcp.TextContent)
	return text.Text
}

func TestCropWatermarkHandler_rejectsTraversalOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "photo.png")
	if err := writeSolidPNG(input, 200, 200); err != nil {
		t.Fatalf("write input: %v", err)
	}

	handler := cropWatermarkHandler(&Config{})
	res, err := handler(context.Background(), cropWatermarkRequest(input, "../../evil.png"))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected traversal output to be rejected, got: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "escapes allowed directories") {
		t.Errorf("error text = %q", resultText(t, res))
	}
}

func TestCropWatermarkHandler_rejectsSymlinkOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "photo.png")
	if err := writeSolidPNG(input, 200, 200); err != nil {
		t.Fatalf("write input: %v", err)
	}
	target := filepath.Join(t.TempDir(), "target.png")
	if err := os.WriteFile(target, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "clean.png")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	handler := cropWatermarkHandler(&Config{})
	res, err := handler(context.Background(), cropWatermarkRequest(input, link))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected symlink output to be rejected, got: %s", resultText(t, res))
	}
	if got, _ := os.ReadFile(target); string(got) != "keep" {
		t.Errorf("symlink target was modified: %q", got)
	}
}

func TestCropWatermarkHandler_allowsOutputInConfiguredDir(t *testing.T) {
	dir := t.TempDir()
	outDir := t.TempDir()
	input := filepath.Join(dir, "photo.png")
	if err := writeSolidPNG(input, 200, 200); err != nil {
		t.Fatalf("write input: %v", err)
	}
	want := filepath.Join(outDir, "clean.png")

	handler := cropWatermarkHandler(&Config{Output: outDir})
	res, err := handler(context.Background(), cropWatermarkRequest(input, want))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected in-root output to be allowed, got: %s", resultText(t, res))
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("output not created: %v", err)
	}
}

func TestCropWatermarkHandler_defaultStaysNextToInput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "photo.png")
	if err := writeSolidPNG(input, 200, 200); err != nil {
		t.Fatalf("write input: %v", err)
	}

	handler := cropWatermarkHandler(&Config{})
	res, err := handler(context.Background(), cropWatermarkRequest(input, ""))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.IsError {
		t.Fatalf("default crop failed: %s", resultText(t, res))
	}
	if _, err := os.Stat(defaultCleanPath(input)); err != nil {
		t.Fatalf("default output not created next to input: %v", err)
	}
}
