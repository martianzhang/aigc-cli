package search

import "context"

// defaultContext carries the CLI cancellation signal into search adapters.
var defaultContext = context.Background()

// SetContext installs the process-wide context used by search adapters.
func SetContext(ctx context.Context) {
	if ctx != nil {
		defaultContext = ctx
	}
}

// Context returns the process-wide search context (never nil).
func Context() context.Context {
	if defaultContext == nil {
		return context.Background()
	}
	return defaultContext
}
