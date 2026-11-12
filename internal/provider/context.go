package provider

import "context"

// defaultContext carries the CLI cancellation signal into provider helpers
// that build their own HTTP requests (online OCR/describe, SFX, SoMark,
// decision). It defaults to Background for tests and library use.
var defaultContext = context.Background()

// SetContext installs the process-wide context used by provider HTTP helpers.
func SetContext(ctx context.Context) {
	if ctx != nil {
		defaultContext = ctx
	}
}

// Context returns the process-wide provider context (never nil).
func Context() context.Context {
	if defaultContext == nil {
		return context.Background()
	}
	return defaultContext
}
