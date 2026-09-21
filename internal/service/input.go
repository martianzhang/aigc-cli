package service

import (
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
