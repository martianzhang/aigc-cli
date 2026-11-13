package cmd

import (
	"context"
	"errors"

	"github.com/martianzhang/aigc-cli/internal/client"
)

// runtimeHint maps common runtime failures to a copy-pasteable next step.
// Usage errors are handled by guidance.Explain; this covers API/auth/timeout
// failures that happen after flags have been parsed.
func runtimeHint(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "request timed out; retry with a larger --timeout or check the HTTP proxy"
	}
	if code, ok := client.StatusCode(err); ok {
		switch code {
		case 401, 403:
			return "check --api-key / --provider / 'aigc-cli --print-config'; do not retry this request"
		case 429:
			return "rate limited; wait for Retry-After or switch provider with --provider"
		}
	}
	return ""
}
