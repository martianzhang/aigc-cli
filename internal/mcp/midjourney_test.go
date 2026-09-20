package mcp

import (
	"slices"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// registryTool returns the toolInfo entry registered under name, or nil.
func registryTool(name string) *toolInfo {
	for i := range toolRegistry {
		if toolRegistry[i].name == name {
			return &toolRegistry[i]
		}
	}
	return nil
}

// assertEnum asserts that a tool property carries exactly the given enum values.
func assertEnum(t *testing.T, tool mcp.Tool, property string, want []string) {
	t.Helper()
	raw, ok := tool.InputSchema.Properties[property]
	if !ok {
		t.Fatalf("tool %s: property %q missing", tool.Name, property)
	}
	schema, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("tool %s: property %q is %T, want map[string]any", tool.Name, property, raw)
	}
	got, ok := schema["enum"].([]string)
	if !ok {
		t.Fatalf("tool %s: property %q enum is %T, want []string", tool.Name, property, schema["enum"])
	}
	if !slices.Equal(got, want) {
		t.Errorf("tool %s: property %q enum = %v, want %v", tool.Name, property, got, want)
	}
}

// ----- toolRegistry entries -----

func TestToolRegistry_hasMidjourneyTools(t *testing.T) {
	want := []string{"midjourney_imagine", "midjourney_describe", "midjourney_reroll", "midjourney_video"}
	for _, name := range want {
		info := registryTool(name)
		if info == nil {
			t.Fatalf("%s not found in toolRegistry", name)
		}
		if info.desc == "" {
			t.Errorf("%s: empty registry description", name)
		}
		if info.newTool == nil {
			t.Errorf("%s: nil newTool builder", name)
		}
		if info.handler == nil {
			t.Errorf("%s: nil handler", name)
		}
		tool := info.newTool(info.desc)
		if tool.Name != name {
			t.Errorf("%s: builder returned tool named %q", name, tool.Name)
		}
		if tool.Description != info.desc {
			t.Errorf("%s: description = %q, want %q", name, tool.Description, info.desc)
		}
	}
}

func TestToolRegistry_imagineWarnsAboutCost(t *testing.T) {
	info := registryTool("midjourney_imagine")
	if info == nil {
		t.Fatal("midjourney_imagine not found in toolRegistry")
	}
	if !contains(info.desc, "prefer generate_image") {
		t.Errorf("midjourney_imagine description should steer agents to generate_image, got %q", info.desc)
	}
}

func TestToolRegistry_noDuplicateNames(t *testing.T) {
	seen := map[string]bool{}
	for _, info := range toolRegistry {
		if seen[info.name] {
			t.Errorf("duplicate tool name in toolRegistry: %s", info.name)
		}
		seen[info.name] = true
	}
}

// ----- tool schemas -----

func TestNewMidjourneyImagineTool(t *testing.T) {
	desc := "Custom midjourney imagine description"
	tool := newMidjourneyImagineTool(desc)

	if tool.Name != "midjourney_imagine" {
		t.Errorf("tool name = %q", tool.Name)
	}
	if tool.Description != desc {
		t.Errorf("description = %q, want %q", tool.Description, desc)
	}
	if !slices.Contains(tool.InputSchema.Required, "prompt") {
		t.Errorf("prompt should be required, got %v", tool.InputSchema.Required)
	}
	for _, property := range []string{"prompt", "image_url", "aspect_ratio", "style", "version", "speed"} {
		if _, ok := tool.InputSchema.Properties[property]; !ok {
			t.Errorf("property %q missing", property)
		}
	}
	assertEnum(t, tool, "aspect_ratio", []string{"1:1", "16:9", "9:16", "4:3", "3:4", "21:9"})
	assertEnum(t, tool, "style", []string{"raw", "expressive"})
	assertEnum(t, tool, "version", []string{"6.1", "7", "8", "8.1"})
	assertEnum(t, tool, "speed", []string{"relax", "fast", "turbo"})
}

func TestNewMidjourneyDescribeTool(t *testing.T) {
	desc := "Custom midjourney describe description"
	tool := newMidjourneyDescribeTool(desc)

	if tool.Name != "midjourney_describe" {
		t.Errorf("tool name = %q", tool.Name)
	}
	if tool.Description != desc {
		t.Errorf("description = %q, want %q", tool.Description, desc)
	}
	if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "image_url" {
		t.Errorf("required = %v, want [image_url]", tool.InputSchema.Required)
	}
	if _, ok := tool.InputSchema.Properties["image_url"]; !ok {
		t.Error("property image_url missing")
	}
}

func TestNewMidjourneyRerollTool(t *testing.T) {
	desc := "Custom midjourney reroll description"
	tool := newMidjourneyRerollTool(desc)

	if tool.Name != "midjourney_reroll" {
		t.Errorf("tool name = %q", tool.Name)
	}
	if tool.Description != desc {
		t.Errorf("description = %q, want %q", tool.Description, desc)
	}
	if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "task_id" {
		t.Errorf("required = %v, want [task_id]", tool.InputSchema.Required)
	}
	if _, ok := tool.InputSchema.Properties["task_id"]; !ok {
		t.Error("property task_id missing")
	}
}

func TestNewMidjourneyVideoTool(t *testing.T) {
	desc := "Custom midjourney video description"
	tool := newMidjourneyVideoTool(desc)

	if tool.Name != "midjourney_video" {
		t.Errorf("tool name = %q", tool.Name)
	}
	if tool.Description != desc {
		t.Errorf("description = %q, want %q", tool.Description, desc)
	}
	if len(tool.InputSchema.Required) != 1 || tool.InputSchema.Required[0] != "image_url" {
		t.Errorf("required = %v, want [image_url]", tool.InputSchema.Required)
	}
	if slices.Contains(tool.InputSchema.Required, "prompt") {
		t.Error("prompt should be optional")
	}
	if _, ok := tool.InputSchema.Properties["prompt"]; !ok {
		t.Error("property prompt missing")
	}
}
