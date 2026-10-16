// Package jsonmerge provides the additive JSON merge used by --inject-json.
package jsonmerge

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Inject merges the inject JSON object into the base JSON body and returns the
// merged document.
//
// Semantics (additive, not a JSON Merge Patch):
//   - objects are merged recursively
//   - arrays are concatenated (base elements first, then inject elements)
//   - every other value in inject overwrites the base value
//
// An empty inject returns base unchanged. Both base (when non-empty) and inject
// must be JSON objects; anything else is an error so a malformed --inject-json
// fails loudly instead of silently producing a wrong body.
func Inject(base, inject json.RawMessage) (json.RawMessage, error) {
	if len(inject) == 0 {
		return base, nil
	}

	inj, err := decodeObject(inject, "inject")
	if err != nil {
		return nil, err
	}
	if len(base) == 0 {
		return encodeObject(inj)
	}

	baseObj, err := decodeObject(base, "body")
	if err != nil {
		return nil, err
	}
	return encodeObject(mergeObjects(baseObj, inj))
}

// decodeObject decodes raw into a JSON object, preserving number precision.
func decodeObject(raw json.RawMessage, what string) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s JSON: %w", what, err)
	}
	if m == nil {
		return nil, fmt.Errorf("%s JSON: not a JSON object", what)
	}
	return m, nil
}

// encodeObject marshals an object with HTML escaping off, matching the rest of
// the request-body pipeline.
func encodeObject(m map[string]any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("encode merged JSON: %w", err)
	}
	return json.RawMessage(bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})), nil
}

// mergeObjects returns a new object with inject layered over base.
func mergeObjects(base, inject map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(inject))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range inject {
		if existing, ok := out[k]; ok {
			out[k] = mergeValue(existing, v)
			continue
		}
		out[k] = v
	}
	return out
}

// mergeValue merges a single value: objects recurse, arrays concatenate, and
// any other value in inject wins.
func mergeValue(base, inject any) any {
	switch iv := inject.(type) {
	case map[string]any:
		if bv, ok := base.(map[string]any); ok {
			return mergeObjects(bv, iv)
		}
		return iv
	case []any:
		if bv, ok := base.([]any); ok {
			out := make([]any, 0, len(bv)+len(iv))
			out = append(out, bv...)
			out = append(out, iv...)
			return out
		}
		return iv
	default:
		return inject
	}
}
