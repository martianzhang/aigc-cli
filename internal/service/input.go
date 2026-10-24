package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// IsFile reports whether path points to an existing regular (non-directory) file.
func IsFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// ReadInput reads content from a file path, stdin ("-"), or returns the raw
// string as bytes.
func ReadInput(input string) ([]byte, error) {
	switch {
	case input == "-":
		stat, err := os.Stdin.Stat()
		if err == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
			return nil, fmt.Errorf("stdin is a terminal — pipe input or use --prompt")
		}
		return io.ReadAll(os.Stdin)
	case IsFile(input):
		return os.ReadFile(input)
	default:
		return []byte(input), nil
	}
}

// ResolvePromptFile returns the contents of input when it names an existing
// file; otherwise it returns input unchanged. Unlike ReadInput it does not
// treat "-" as stdin, so a literal "-" keeps its old meaning.
func ResolvePromptFile(input string) (string, error) {
	if input == "" || !IsFile(input) {
		return input, nil
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file %q: %w", input, err)
	}
	return string(data), nil
}

// ReadJSONInput resolves a JSON payload from stdin ("-"), a file path, or an
// inline JSON literal (must start with '{', '[' or '"'). A value that is none
// of those is reported as a missing file, so a mistyped path says
// "file not found" instead of failing later in the JSON parser.
func ReadJSONInput(input string) ([]byte, error) {
	if input == "-" || IsFile(input) || isInlineJSON(input) {
		data, err := ReadInput(input)
		if err != nil {
			return nil, err
		}
		return NormalizeJSONC(data), nil
	}
	return nil, fmt.Errorf("file not found: %s (pass a file path, inline JSON, or \"-\" for stdin)", input)
}

// ReadInjectJSON resolves a --inject-json value (a file path or inline JSON) and
// validates that it is a JSON object. An empty input yields nil so callers can
// treat "not provided" and "empty object" the same way.
//
// Unlike --json, --inject-json deliberately does not read stdin ("-"): the
// snippet is small, and stdin is a single-consumer resource that other inputs
// (--json, --prompt) may already own.
func ReadInjectJSON(input string) (json.RawMessage, error) {
	if input == "" {
		return nil, nil
	}
	if input == "-" {
		return nil, fmt.Errorf("--inject-json does not read stdin; pass a file path or an inline JSON object")
	}
	if !IsFile(input) && !isInlineJSON(input) {
		return nil, fmt.Errorf("file not found: %s (pass a file path or an inline JSON object)", input)
	}
	data, err := ReadInput(input)
	if err != nil {
		return nil, err
	}
	data = NormalizeJSONC(data)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid --inject-json: %w", err)
	}
	if m == nil {
		return nil, fmt.Errorf("invalid --inject-json: not a JSON object")
	}
	return json.RawMessage(data), nil
}

// isInlineJSON reports whether input looks like an inline JSON document rather
// than a file path. Leading whitespace is ignored. A JSONC document may open
// with a comment, so a value starting with "//" or "/*" is treated as inline.
func isInlineJSON(input string) bool {
	trimmed := strings.TrimLeft(input, " \t\r\n")
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case '{', '[', '"':
		return true
	case '/':
		return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*")
	default:
		return false
	}
}
