package client

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestAPIStatusError(t *testing.T) {
	err := &APIStatusError{StatusCode: http.StatusBadRequest, Body: `{"error":"bad size"}`}
	if got, want := err.Error(), `API returned status 400: {"error":"bad size"}`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestStatusCode(t *testing.T) {
	base := &APIStatusError{StatusCode: http.StatusBadRequest, Body: "bad"}
	wrapped := fmt.Errorf("image generation failed: %w", base)

	code, ok := StatusCode(wrapped)
	if !ok || code != http.StatusBadRequest {
		t.Errorf("StatusCode(wrapped) = (%d, %v), want (%d, true)", code, ok, http.StatusBadRequest)
	}

	if _, ok := StatusCode(errors.New("plain error")); ok {
		t.Error("StatusCode(plain) ok = true, want false")
	}
}
