package cliutil

import "strings"

// FirstNonBlank returns the first value that is non-empty after TrimSpace.
// Unlike cmp.Or, a whitespace-only value is skipped.
func FirstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
