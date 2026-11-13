package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
)

func TestRuntimeHintStatusCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"unauthorized", &client.APIStatusError{StatusCode: 401}, "check --api-key"},
		{"forbidden", &client.APIStatusError{StatusCode: 403}, "check --api-key"},
		{"rate limited", &client.APIStatusError{StatusCode: 429}, "rate limited"},
		{"other", &client.APIStatusError{StatusCode: 500}, ""},
		{"timeout", context.DeadlineExceeded, "request timed out"},
		{"plain", errors.New("boom"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runtimeHint(tt.err)
			if tt.want == "" {
				if got != "" {
					t.Fatalf("runtimeHint() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("runtimeHint() = %q, want %q", got, tt.want)
			}
		})
	}
}
