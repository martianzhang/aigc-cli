package service

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// scanForC2PA does a raw byte scan for JUMBF/C2PA signatures.
// Useful for formats like WebP, GIF, BMP that the library doesn't support.
func scanForC2PA(r io.Reader) []byte {
	data, err := io.ReadAll(r)
	if err != nil || len(data) == 0 {
		return nil
	}
	// JUMBF boxes start with "jumb" (4-byte magic)
	// C2PA uses JUMBF as its container format — if we find it, C2PA is present.
	idx := bytes.Index(data, []byte("jumb"))
	if idx < 0 {
		idx = bytes.Index(data, []byte("C2PA"))
	}
	if idx < 0 {
		// China TC260 AIGC label (GB 45438-2025)
		idx = bytes.Index(data, []byte("TC260:AIGC"))
	}
	if idx < 0 {
		// CNIPA / Chinese digital watermark standards
		idx = bytes.Index(data, []byte("<TC260"))
	}
	if idx < 0 {
		return nil
	}
	// Return surrounding bytes for string extraction
	start := idx
	if start > 64 {
		start -= 64
	}
	end := idx + 4096
	if end > len(data) {
		end = len(data)
	}
	return data[start:end]
}

// truncateMeta shortens long metadata values for display.
func truncateMeta(s string) string {
	// Collapse whitespace
	fields := strings.Fields(s)
	s = strings.Join(fields, " ")
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// humanSize formats a byte count as a human-readable string.
func humanSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// camelToWords converts "trainedAlgorithmicMedia" to "Trained Algorithmic Media".
func camelToWords(s string) string {
	if s == "" {
		return ""
	}
	var words []string
	var cur []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if len(cur) > 0 {
				words = append(words, string(cur))
			}
			cur = []rune{r}
		} else {
			cur = append(cur, r)
		}
	}
	if len(cur) > 0 {
		words = append(words, string(cur))
	}
	return strings.Join(words, " ")
}

// hasControlChars returns true if the string contains control characters (except null).
func hasControlChars(s string) bool {
	for _, r := range s {
		if r > 0 && r < 32 {
			return true
		}
	}
	return false
}
