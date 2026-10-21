package webfetch

import (
	"strings"
	"testing"
)

func TestParseHeader(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantName  string
		wantValue string
		wantError string
	}{
		{name: "value", line: "Authorization: Bearer abc", wantName: "Authorization", wantValue: "Bearer abc"},
		{name: "extra colon", line: "X-Test: a:b", wantName: "X-Test", wantValue: "a:b"},
		{name: "remove", line: "Accept:", wantName: "Accept", wantValue: ""},
		{name: "no colon", line: "Broken", wantError: "expected"},
		{name: "empty name", line: ": value", wantError: "name is required"},
		{name: "crlf value", line: "X-Test: a\r\nInjected: b", wantError: "CR/LF"},
		{name: "invalid name", line: "Bad Name: x", wantError: "invalid name"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, err := ParseHeader(tc.line)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want it to contain %q", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseHeader: %v", err)
			}
			if h.Name != tc.wantName || h.Value != tc.wantValue {
				t.Errorf("header = %+v, want %s=%q", h, tc.wantName, tc.wantValue)
			}
		})
	}
}

func TestEffectiveHeaders(t *testing.T) {
	headers := EffectiveHeadersWithDefaults(
		[]Header{
			{Name: "User-Agent", Value: "default-agent"},
			{Name: "Accept", Value: "text/html"},
			{Name: "Accept-Language", Value: "en"},
		},
		[]Header{
			{Name: "user-agent", Value: "custom-agent"},
			{Name: "Accept", Value: ""},
			{Name: "X-Test", Value: "1"},
		},
	)

	got := make(map[string]string, len(headers))
	for _, h := range headers {
		got[h.Name] = h.Value
	}
	if got["User-Agent"] != "custom-agent" {
		t.Errorf("User-Agent = %q, want custom-agent", got["User-Agent"])
	}
	if _, ok := got["Accept"]; ok {
		t.Errorf("Accept = %q, want removed", got["Accept"])
	}
	if got["Accept-Language"] != "en" || got["X-Test"] != "1" {
		t.Errorf("headers = %+v, want defaults plus X-Test", got)
	}
}
