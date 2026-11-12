// Package argutil provides shell-like argument parsing for CLI flags that
// accept a raw ffmpeg-style argument string.
package argutil

import "strings"

// Split splits a space-separated argument string, honoring double quotes.
// Empty quoted strings are preserved; unquoted whitespace separates arguments.
func Split(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var args []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}
