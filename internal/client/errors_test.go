package client

import (
	"net/http"
	"strings"
	"testing"
)

func TestSanitizeErrorBodyRedactsSecrets(t *testing.T) {
	body := `{"error":{"message":"unauthorized"},"api_key":"sk-live-1234567890abcdef","access_token":"tok-abcdef123456"}`
	got := sanitizeErrorBody(body)
	for _, leaked := range []string{"sk-live-1234567890abcdef", "tok-abcdef123456"} {
		if strings.Contains(got, leaked) {
			t.Errorf("sanitized body still contains %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, "unauthorized") {
		t.Errorf("sanitized body dropped the useful message: %s", got)
	}
}

func TestSanitizeErrorBodyRedactsQueryAndBearer(t *testing.T) {
	body := "GET https://cdn.example.com/a.png?token=abc123&signature=deadbeef failed; Authorization: Bearer sk-or-secret-value"
	got := sanitizeErrorBody(body)
	for _, leaked := range []string{"abc123", "deadbeef", "sk-or-secret-value"} {
		if strings.Contains(got, leaked) {
			t.Errorf("sanitized body still contains %q: %s", leaked, got)
		}
	}
	if !strings.Contains(got, "cdn.example.com") {
		t.Errorf("sanitized body lost the host: %s", got)
	}
}

func TestSanitizeErrorBodyTruncates(t *testing.T) {
	body := strings.Repeat("x", maxErrorBodyBytes*2)
	got := sanitizeErrorBody(body)
	if len(got) > maxErrorBodyBytes+64 {
		t.Fatalf("sanitized body length = %d, want about %d", len(got), maxErrorBodyBytes)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("truncated body has no marker: %s", got[len(got)-40:])
	}
}

func TestSanitizeErrorBodyKeepsShortJSON(t *testing.T) {
	body := `{"error":{"message":"Invalid size '16:16'."}}`
	if got := sanitizeErrorBody(body); got != body {
		t.Fatalf("short JSON changed: %q -> %q", body, got)
	}
}

func TestNewAPIStatusErrorSanitizesBody(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusUnauthorized}
	err := newAPIStatusError(resp, []byte(`{"api_key":"sk-abcdefgh12345678"}`))
	if strings.Contains(err.Body, "sk-abcdefgh12345678") {
		t.Fatalf("APIStatusError.Body leaked the key: %s", err.Body)
	}
}
