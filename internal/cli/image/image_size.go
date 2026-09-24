package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// splitSizeTier splits a compound "<tier>@<ratio>" size value (e.g. "2K@16:9")
// on the first "@". Values without "@" are returned unchanged, so existing
// sizes ("16:9", "1024x1024", "2K") stay a pure no-op.
func splitSizeTier(v string) (size, ratio string, err error) {
	if !strings.Contains(v, "@") {
		return v, "", nil
	}
	parts := strings.SplitN(v, "@", 2)
	size = strings.TrimSpace(parts[0])
	ratio = strings.TrimSpace(parts[1])
	if size == "" || ratio == "" || strings.Contains(ratio, "@") {
		return "", "", fmt.Errorf("invalid --size %q: expected \"<tier>@<ratio>\" (e.g. \"2K@16:9\")", v)
	}
	return size, ratio, nil
}

// normalizeSizeTier expands a compound "<tier>@<ratio>" size into the separate
// Size and Ratio request fields, then applies the same split to a verbatim
// --json body's top-level "size" string. Values without "@" are a no-op.
func normalizeSizeTier(req *types.GenerateRequest) error {
	if !strings.Contains(req.Size, "@") {
		return nil
	}
	original := req.Size
	size, ratio, err := splitSizeTier(original)
	if err != nil {
		return err
	}
	if req.Ratio != "" && req.Ratio != ratio {
		return fmt.Errorf("conflicting ratio: size %q implies %q but ratio is already %q", original, ratio, req.Ratio)
	}
	req.Size = size
	req.Ratio = ratio
	return normalizeRawSizeTier(req)
}

// normalizeRawSizeTier rewrites a verbatim --json body's top-level "size" into
// separate "size"/"ratio" keys. It mirrors resolveRawImagePaths' fidelity
// guarantees: json.Number decoding, no HTML escaping, no trailing newline.
// Bodies without a compound "size" are left byte-identical.
func normalizeRawSizeTier(req *types.GenerateRequest) error {
	if len(req.RawJSON) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(req.RawJSON))
	dec.UseNumber()
	var body map[string]interface{}
	if err := dec.Decode(&body); err != nil {
		return fmt.Errorf("failed to parse JSON body: %w", err)
	}
	rawSize, ok := body["size"].(string)
	if !ok || !strings.Contains(rawSize, "@") {
		return nil
	}
	size, ratio, err := splitSizeTier(rawSize)
	if err != nil {
		return err
	}
	if existing, ok := body["ratio"].(string); ok && existing != ratio {
		return fmt.Errorf("conflicting ratio in JSON body: size %q implies %q but ratio is %q", rawSize, ratio, existing)
	}
	body["size"] = size
	body["ratio"] = ratio
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return fmt.Errorf("failed to encode JSON body: %w", err)
	}
	req.RawJSON = json.RawMessage(bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}))
	return nil
}
