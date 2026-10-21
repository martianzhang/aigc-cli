package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/martianzhang/aigc-cli/internal/jsonmerge"
)

// Options carries per-request vendor overrides, normally from --inject-json.
// Extra is a JSON object; empty/nil means no overrides.
type Options struct {
	Extra json.RawMessage
}

// mergePayload marshals the base vendor payload and layers Extra on top using
// the same additive rules as every other --inject-json path (objects recurse,
// arrays concatenate, scalars overwrite).
func mergePayload(payload map[string]any, opts Options) (json.RawMessage, error) {
	base, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode search payload: %w", err)
	}
	merged, err := jsonmerge.Inject(base, opts.Extra)
	if err != nil {
		return nil, fmt.Errorf("inject search payload: %w", err)
	}
	return merged, nil
}

// injectQuery appends Extra to a GET request's query string. GET vendors only
// accept flat parameters, so nested objects and arrays of objects are rejected
// instead of silently sending something the vendor cannot interpret.
func injectQuery(rawURL string, opts Options) (string, error) {
	if len(opts.Extra) == 0 {
		return rawURL, nil
	}

	var extra map[string]any
	dec := json.NewDecoder(bytes.NewReader(opts.Extra))
	dec.UseNumber()
	if err := dec.Decode(&extra); err != nil {
		return "", fmt.Errorf("inject JSON: %w", err)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse request URL: %w", err)
	}
	values := u.Query()
	for key, value := range extra {
		switch v := value.(type) {
		case string:
			values.Set(key, v)
		case json.Number:
			values.Set(key, v.String())
		case bool:
			values.Set(key, strconv.FormatBool(v))
		case []any:
			for _, item := range v {
				s, err := scalarString(item)
				if err != nil {
					return "", fmt.Errorf("inject JSON key %q: %w", key, err)
				}
				values.Add(key, s)
			}
		default:
			return "", fmt.Errorf("inject JSON key %q: GET providers only accept scalar values or arrays of scalars", key)
		}
	}
	u.RawQuery = values.Encode()
	return u.String(), nil
}

// scalarString converts one JSON scalar into a query-string value.
func scalarString(v any) (string, error) {
	switch s := v.(type) {
	case string:
		return s, nil
	case json.Number:
		return s.String(), nil
	case bool:
		return strconv.FormatBool(s), nil
	default:
		return "", fmt.Errorf("value must be a string, number or boolean")
	}
}
