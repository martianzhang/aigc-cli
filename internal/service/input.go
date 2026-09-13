package service

import (
	"fmt"
	"io"
	"os"
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
