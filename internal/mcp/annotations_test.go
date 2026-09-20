package mcp

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// annotationWant is one row of the expected tool → annotation mapping.
type annotationWant struct {
	tool        string
	readOnly    bool
	destructive bool
	idempotent  bool
	openWorld   bool
}

// annotationTable is the single source of truth for every tool's hints.
// Every tool in toolRegistry must appear exactly once.
var annotationTable = []annotationWant{
	// Pure local reads: no network, no writes.
	{"detect_image", true, false, false, false},
	{"recognize_text", true, false, false, false},
	{"search_ideas", true, false, false, false},
	{"caption_image", true, false, false, false},
	{"kb_find", true, false, false, false},
	{"kb_list", true, false, false, false},
	{"kb_show", true, false, false, false},
	// Network reads.
	{"list_models", true, false, false, true},
	{"get_model_pricing", true, false, false, true},
	{"get_config", true, false, false, true},
	// Network reads whose repeated calls add no effect.
	{"get_balance", true, false, true, true},
	{"get_task", true, false, true, true},
	// Tools that create new output files/entries and never overwrite inputs.
	{"generate_image", false, false, false, true},
	{"generate_video", false, false, false, true},
	{"generate_music", false, false, false, true},
	{"generate_speech", false, false, false, true},
	{"transcribe_audio", false, false, false, true},
	{"midjourney_imagine", false, false, false, true},
	{"midjourney_describe", false, false, false, true},
	{"midjourney_reroll", false, false, false, true},
	{"midjourney_video", false, false, false, true},
	{"kb_add", false, false, false, true},
	{"kb_fetch", false, false, false, true},
	{"kb_search", false, false, false, true},
	{"remove_background", false, false, false, true},
	{"convert_depth", false, false, false, true},
	{"remove_watermark", false, false, false, true},
	{"crop_watermark", false, false, false, true},
	{"add_watermark", false, false, false, true},
}

func Test_toolAnnotations_matchTable(t *testing.T) {
	registered := make(map[string]mcp.Tool, len(toolRegistry))
	for _, info := range toolRegistry {
		registered[info.name] = info.newTool(info.desc)
	}
	if len(annotationTable) != len(registered) {
		t.Fatalf("annotation table has %d rows, tool registry has %d tools", len(annotationTable), len(registered))
	}

	seen := make(map[string]bool, len(annotationTable))
	for _, want := range annotationTable {
		if seen[want.tool] {
			t.Fatalf("duplicate annotation table row for %q", want.tool)
		}
		seen[want.tool] = true

		tool, ok := registered[want.tool]
		if !ok {
			t.Errorf("annotation table lists %q, which is not in the tool registry", want.tool)
			continue
		}
		assertAnnotations(t, tool, want)
	}
	for name := range registered {
		if !seen[name] {
			t.Errorf("tool %q is registered but has no annotation table row", name)
		}
	}
}

func assertAnnotations(t *testing.T, tool mcp.Tool, want annotationWant) {
	t.Helper()
	ann := tool.Annotations
	if ann.ReadOnlyHint == nil || ann.DestructiveHint == nil || ann.IdempotentHint == nil || ann.OpenWorldHint == nil {
		t.Errorf("%s: all four hints must be explicit, got %+v", tool.Name, ann)
		return
	}
	if got := *ann.ReadOnlyHint; got != want.readOnly {
		t.Errorf("%s: readOnlyHint = %t, want %t", tool.Name, got, want.readOnly)
	}
	if got := *ann.DestructiveHint; got != want.destructive {
		t.Errorf("%s: destructiveHint = %t, want %t", tool.Name, got, want.destructive)
	}
	if got := *ann.IdempotentHint; got != want.idempotent {
		t.Errorf("%s: idempotentHint = %t, want %t", tool.Name, got, want.idempotent)
	}
	if got := *ann.OpenWorldHint; got != want.openWorld {
		t.Errorf("%s: openWorldHint = %t, want %t", tool.Name, got, want.openWorld)
	}
}

// Test_registeredToolAnnotations_neverDestructive exercises the tools NewServer
// actually registers: every hint must survive into the marshaled tools/list
// payload, and no tool may ever declare destructiveHint: true.
func Test_registeredToolAnnotations_neverDestructive(t *testing.T) {
	cfg := &Config{
		APIKey:   "sk-test",
		BaseURL:  "https://api.openai.com/v1",
		Defaults: &types.ConfigDefaults{},
	}
	tools := NewServer(cfg).ListTools()
	if len(tools) != len(toolRegistry) {
		t.Fatalf("registered %d tools, want %d", len(tools), len(toolRegistry))
	}

	hints := []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"}
	for name, st := range tools {
		ann := st.Tool.Annotations
		if ann.DestructiveHint == nil {
			t.Errorf("%s: destructiveHint must be explicit", name)
			continue
		}
		if *ann.DestructiveHint {
			t.Errorf("%s: destructiveHint must never be true", name)
		}

		payload, err := json.Marshal(st.Tool)
		if err != nil {
			t.Fatalf("%s: marshal tool: %v", name, err)
		}
		var raw struct {
			Annotations map[string]bool `json:"annotations"`
		}
		if err := json.Unmarshal(payload, &raw); err != nil {
			t.Fatalf("%s: unmarshal tool: %v", name, err)
		}
		for _, hint := range hints {
			if _, ok := raw.Annotations[hint]; !ok {
				t.Errorf("%s: %s missing from marshaled tool", name, hint)
			}
		}
	}
}
