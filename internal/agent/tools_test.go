package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// toolNamePattern matches OpenAI-compatible function names (lowercase snake_case).
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// expectedToolNames is the golden set of tool names exposed to the LLM.
// Adding or removing a tool in ToolDefs requires updating this list (and docs).
var expectedToolNames = []string{
	"add_watermark",
	"balance",
	"caption_image",
	"convert_depth",
	"find",
	"generate_image",
	"generate_music",
	"generate_speech",
	"generate_video",
	"grep",
	"kb_add",
	"kb_fetch",
	"kb_find",
	"kb_list",
	"kb_search",
	"kb_show",
	"midjourney_describe",
	"midjourney_imagine",
	"midjourney_reroll",
	"midjourney_video",
	"read_file",
	"recognize_text",
	"remove_background",
	"remove_watermark",
	"search_ideas",
	"task",
	"transcribe_audio",
	"web_fetch",
}

// decodeToolSchema parses a tool definition's Parameters into a generic schema map.
func decodeToolSchema(t *testing.T, def types.ToolDefinition) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(def.Function.Parameters, &schema); err != nil {
		t.Fatalf("%s: Parameters is not valid JSON: %v", def.Function.Name, err)
	}
	return schema
}

func TestToolDefsTopLevelShape(t *testing.T) {
	if len(ToolDefs) == 0 {
		t.Fatal("ToolDefs is empty")
	}
	for i, def := range ToolDefs {
		t.Run(fmt.Sprintf("%02d_%s", i, def.Function.Name), func(t *testing.T) {
			if def.Type != "function" {
				t.Errorf("Type = %q, want %q", def.Type, "function")
			}
			if def.Function.Name == "" {
				t.Fatal("Function.Name is empty")
			}
			if !toolNamePattern.MatchString(def.Function.Name) {
				t.Errorf("Function.Name = %q, want lowercase snake_case", def.Function.Name)
			}
			if desc := strings.TrimSpace(def.Function.Description); len(desc) < 20 {
				t.Errorf("Function.Description too short (%d chars), the LLM needs it to pick the tool: %q", len(desc), desc)
			}
			if len(def.Function.Parameters) == 0 {
				t.Error("Function.Parameters is empty")
			}
		})
	}
}

func TestToolDefsNamesUnique(t *testing.T) {
	seen := make(map[string]int, len(ToolDefs))
	for i, def := range ToolDefs {
		name := def.Function.Name
		if prev, ok := seen[name]; ok {
			t.Errorf("duplicate tool name %q at index %d (first seen at index %d)", name, i, prev)
		}
		seen[name] = i
	}
}

func TestToolDefsGoldenNames(t *testing.T) {
	got := make(map[string]bool, len(ToolDefs))
	for _, def := range ToolDefs {
		got[def.Function.Name] = true
	}
	var missing, extra []string
	for _, name := range expectedToolNames {
		if !got[name] {
			missing = append(missing, name)
		}
		delete(got, name)
	}
	for name := range got {
		extra = append(extra, name)
	}
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("tool surface changed: missing=%v extra=%v — update expectedToolNames deliberately", missing, extra)
	}
}

func TestToolDefsParameterSchemas(t *testing.T) {
	for _, def := range ToolDefs {
		t.Run(def.Function.Name, func(t *testing.T) {
			schema := decodeToolSchema(t, def)
			if schema["type"] != "object" {
				t.Errorf("schema type = %v, want \"object\"", schema["type"])
			}
			props, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("properties missing or not an object: %T", schema["properties"])
			}
			checkRequiredSubset(t, schema, props)
			checkProperties(t, props)
		})
	}
}

// checkRequiredSubset verifies every required name is declared in properties —
// a required key without a property definition breaks tool calling.
func checkRequiredSubset(t *testing.T, schema map[string]any, props map[string]any) {
	t.Helper()
	rawReq, ok := schema["required"]
	if !ok {
		return
	}
	reqList, ok := rawReq.([]any)
	if !ok {
		t.Errorf("required is not an array: %T", rawReq)
		return
	}
	for _, rv := range reqList {
		name, ok := rv.(string)
		if !ok {
			t.Errorf("required entry is not a string: %T", rv)
			continue
		}
		if _, ok := props[name]; !ok {
			t.Errorf("required parameter %q is not defined in properties", name)
		}
	}
}

// checkProperties verifies per-property type, enum integrity and numeric bounds.
func checkProperties(t *testing.T, props map[string]any) {
	t.Helper()
	for name, raw := range props {
		prop, ok := raw.(map[string]any)
		if !ok {
			t.Errorf("property %q is not an object: %T", name, raw)
			continue
		}
		if tp, _ := prop["type"].(string); tp == "" {
			t.Errorf("property %q has no \"type\"", name)
		}
		if rawEnum, ok := prop["enum"]; ok {
			enum, ok := rawEnum.([]any)
			if !ok || len(enum) == 0 {
				t.Errorf("property %q enum is not a non-empty array", name)
				continue
			}
			seen := make(map[string]bool, len(enum))
			for _, v := range enum {
				key := fmt.Sprint(v)
				if seen[key] {
					t.Errorf("property %q enum has duplicate value %v", name, v)
				}
				seen[key] = true
			}
		}
		minVal, hasMin := prop["minimum"].(float64)
		maxVal, hasMax := prop["maximum"].(float64)
		if hasMin && hasMax && minVal > maxVal {
			t.Errorf("property %q has minimum %v > maximum %v", name, minVal, maxVal)
		}
	}
}

func TestToolDefsJSONWireFormat(t *testing.T) {
	data, err := json.Marshal(ToolDefs)
	if err != nil {
		t.Fatalf("marshal ToolDefs: %v", err)
	}
	var rawDefs []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawDefs); err != nil {
		t.Fatalf("unmarshal ToolDefs: %v", err)
	}
	if len(rawDefs) != len(ToolDefs) {
		t.Fatalf("round-trip count = %d, want %d", len(rawDefs), len(ToolDefs))
	}
	for i, raw := range rawDefs {
		name := ToolDefs[i].Function.Name
		if _, ok := raw["type"]; !ok {
			t.Errorf("%s: top-level \"type\" key missing", name)
		}
		fnRaw, ok := raw["function"]
		if !ok {
			t.Errorf("%s: top-level \"function\" key missing", name)
			continue
		}
		var fn map[string]json.RawMessage
		if err := json.Unmarshal(fnRaw, &fn); err != nil {
			t.Errorf("%s: function is not an object: %v", name, err)
			continue
		}
		for _, key := range []string{"name", "description", "parameters"} {
			if _, ok := fn[key]; !ok {
				t.Errorf("%s: function.%s key missing", name, key)
			}
		}
		// parameters must be a bare JSON object, never a double-encoded string.
		var params map[string]any
		if err := json.Unmarshal(fn["parameters"], &params); err != nil {
			t.Errorf("%s: parameters is not a bare JSON object: %v", name, err)
		}
	}
}

func TestToolDefsJSONRoundTrip(t *testing.T) {
	data, err := json.Marshal(ToolDefs)
	if err != nil {
		t.Fatalf("marshal ToolDefs: %v", err)
	}
	var decoded []types.ToolDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal ToolDefs: %v", err)
	}
	if len(decoded) != len(ToolDefs) {
		t.Fatalf("round-trip count = %d, want %d", len(decoded), len(ToolDefs))
	}
	for i := range decoded {
		want, got := ToolDefs[i], decoded[i]
		if want.Type != got.Type || want.Function.Name != got.Function.Name ||
			want.Function.Description != got.Function.Description {
			t.Errorf("def[%d] metadata changed in round-trip: %+v → %+v", i, want, got)
		}
		// RawMessage whitespace differs after compaction, compare semantically.
		var wantParams, gotParams any
		if err := json.Unmarshal(want.Function.Parameters, &wantParams); err != nil {
			t.Fatalf("%s: unmarshal original Parameters: %v", want.Function.Name, err)
		}
		if err := json.Unmarshal(got.Function.Parameters, &gotParams); err != nil {
			t.Fatalf("%s: unmarshal round-tripped Parameters: %v", want.Function.Name, err)
		}
		if !reflect.DeepEqual(wantParams, gotParams) {
			t.Errorf("%s: Parameters changed in round-trip", want.Function.Name)
		}
	}
}

// TestToolCallFunctionArgumentsRoundTrip locks the arguments contract shared with
// the invocation side: the LLM returns tool call arguments as a JSON string that
// handlers decode into the very properties declared by ToolDefs.
// (There is no ToolResult type in this repo; ToolCall/ToolCallFunction is the
// actual struct pair carrying a tool invocation back to the agent.)
func TestToolCallFunctionArgumentsRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		call types.ToolCall
	}{
		{
			name: "generate_image",
			call: types.ToolCall{
				ID:   "call_1",
				Type: "function",
				Function: types.ToolCallFunction{
					Name:      "generate_image",
					Arguments: `{"prompt":"a cat under the stars","n":2}`,
				},
			},
		},
		{
			name: "kb_list_no_args",
			call: types.ToolCall{
				ID:       "call_2",
				Type:     "function",
				Function: types.ToolCallFunction{Name: "kb_list", Arguments: "{}"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := json.Marshal(c.call)
			if err != nil {
				t.Fatalf("marshal ToolCall: %v", err)
			}
			var decoded types.ToolCall
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("unmarshal ToolCall: %v", err)
			}
			if decoded.ID != c.call.ID || decoded.Type != c.call.Type ||
				decoded.Function.Name != c.call.Function.Name {
				t.Errorf("round-trip changed metadata: %+v → %+v", c.call, decoded)
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(decoded.Function.Arguments), &args); err != nil {
				t.Fatalf("Function.Arguments is not valid JSON: %v", err)
			}
			var wantArgs map[string]any
			if err := json.Unmarshal([]byte(c.call.Function.Arguments), &wantArgs); err != nil {
				t.Fatalf("fixture arguments invalid: %v", err)
			}
			if !reflect.DeepEqual(wantArgs, args) {
				t.Errorf("arguments changed: %v → %v", wantArgs, args)
			}
		})
	}
}
