package client

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// maxErrorBodyBytes caps the upstream body embedded in an error message. Error
// bodies are often an HTML page from a proxy or CDN; printing the whole thing to
// a terminal is noisy and can leak data from the request.
const maxErrorBodyBytes = 2048

// APIStatusError is returned when an API responds with a non-2xx status.
// Its Error text matches the historical "API returned status %d: %s" format,
// so existing callers and tests are unaffected; the structured fields let
// callers add provider-specific guidance (e.g. --size hints) on failures.
// Body is sanitized by newAPIStatusError: truncated and stripped of obvious
// credentials before it reaches the terminal.
type APIStatusError struct {
	StatusCode int
	Body       string
}

func (e *APIStatusError) Error() string {
	return fmt.Sprintf("API returned status %d: %s", e.StatusCode, e.Body)
}

// newAPIStatusError builds an APIStatusError from an HTTP response.
func newAPIStatusError(resp *http.Response, body []byte) *APIStatusError {
	return &APIStatusError{StatusCode: resp.StatusCode, Body: sanitizeErrorBody(string(body))}
}

// StatusCode extracts the HTTP status code from an error chain, if present.
func StatusCode(err error) (int, bool) {
	var e *APIStatusError
	if errors.As(err, &e) {
		return e.StatusCode, true
	}
	return 0, false
}

// Sensitive URL query parameters and JSON/YAML fields commonly echoed by
// gateways (signed URLs, proxy diagnostics, auth errors).
var (
	errorQuerySecret = regexp.MustCompile(`(?i)([?&](?:key|api_?key|token|access_?token|api-key|signature|sig|secret)=)[^&\s"'<>]+`)
	errorFieldSecret = regexp.MustCompile(`(?i)("?(?:api_?key|access_?token|authorization|secret|password)"?\s*[:=]\s*"?)[^",&\s}]+`)
	errorBearer      = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`)
	errorSkKey       = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`)
)

const redacted = "***"

// sanitizeErrorBody makes an upstream response body safe to print: secrets in
// query strings, JSON fields and Authorization-style values are redacted, then
// the result is truncated. Truncation happens after redaction so a secret that
// straddles the cut still cannot leak.
func sanitizeErrorBody(body string) string {
	cleaned := strings.TrimSpace(body)
	if cleaned == "" {
		return cleaned
	}
	cleaned = errorQuerySecret.ReplaceAllString(cleaned, "${1}"+redacted)
	cleaned = errorFieldSecret.ReplaceAllString(cleaned, "${1}"+redacted)
	cleaned = errorBearer.ReplaceAllString(cleaned, "${1}"+redacted)
	cleaned = errorSkKey.ReplaceAllString(cleaned, redacted)

	if len(cleaned) > maxErrorBodyBytes {
		// Cut on a rune boundary so the terminal never receives a broken
		// multi-byte sequence.
		cut := maxErrorBodyBytes
		for cut > 0 && !utf8Start(cleaned[cut]) {
			cut--
		}
		cleaned = cleaned[:cut] + fmt.Sprintf("… (truncated, %d bytes)", len(body))
	}
	return cleaned
}

// utf8Start reports whether b starts a UTF-8 rune (not a continuation byte).
func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
