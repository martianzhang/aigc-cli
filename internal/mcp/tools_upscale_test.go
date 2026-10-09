package mcp

import "testing"

func TestUpscaleToolSchema(t *testing.T) {
	tool := newUpscaleTool()
	if tool.Name != "upscale" {
		t.Fatalf("tool name = %q, want upscale", tool.Name)
	}
	for _, p := range []string{"input_path", "output_path", "model", "scale"} {
		if _, ok := tool.InputSchema.Properties[p]; !ok {
			t.Errorf("upscale schema missing %q", p)
		}
	}
}
