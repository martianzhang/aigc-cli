package client

import (
	"errors"
	"fmt"
	"net/http"
)

// APIStatusError is returned when an API responds with a non-2xx status.
// Its Error text matches the historical "API returned status %d: %s" format,
// so existing callers and tests are unaffected; the structured fields let
// callers add provider-specific guidance (e.g. --size hints) on failures.
type APIStatusError struct {
	StatusCode int
	Body       string
}

func (e *APIStatusError) Error() string {
	return fmt.Sprintf("API returned status %d: %s", e.StatusCode, e.Body)
}

// newAPIStatusError builds an APIStatusError from an HTTP response.
func newAPIStatusError(resp *http.Response, body []byte) *APIStatusError {
	return &APIStatusError{StatusCode: resp.StatusCode, Body: string(body)}
}

// StatusCode extracts the HTTP status code from an error chain, if present.
func StatusCode(err error) (int, bool) {
	var e *APIStatusError
	if errors.As(err, &e) {
		return e.StatusCode, true
	}
	return 0, false
}
