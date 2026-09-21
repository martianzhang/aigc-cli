package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// MergeJSONOverlay returns raw with every key in set written into it.
// When set is empty it returns raw unchanged (byte-identical), so a request
// built from --json alone is never re-encoded. Keys may use dotted paths
// (e.g. "extra_body.image") to write nested objects. Raw must be a JSON object.
func MergeJSONOverlay(raw json.RawMessage, set map[string]any) (json.RawMessage, error) {
	if len(set) == 0 {
		return raw, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return nil, fmt.Errorf("decode JSON body: %w", err)
	}
	if body == nil {
		return nil, fmt.Errorf("decode JSON body: not a JSON object")
	}

	for key, value := range set {
		if err := setOverlayPath(body, key, value); err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return nil, fmt.Errorf("encode JSON body: %w", err)
	}
	return json.RawMessage(bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})), nil
}

// setOverlayPath writes value at a dotted key, creating intermediate objects.
func setOverlayPath(body map[string]any, key string, value any) error {
	parts := strings.Split(key, ".")
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("invalid overlay key %q", key)
		}
	}

	cur := body
	for _, part := range parts[:len(parts)-1] {
		switch child := cur[part].(type) {
		case nil:
			next := make(map[string]any)
			cur[part] = next
			cur = next
		case map[string]any:
			cur = child
		default:
			return fmt.Errorf("overlay key %q: %q is not an object", key, part)
		}
	}
	cur[parts[len(parts)-1]] = value
	return nil
}
