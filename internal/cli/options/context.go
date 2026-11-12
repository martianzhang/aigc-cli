package options

import "context"

// defaultContext carries the process-wide cancellation signal. The CLI sets it
// once at startup (signal.NotifyContext) so clients created via NewClient honor
// Ctrl-C without threading a context through every command signature.
var defaultContext = context.Background()

// SetContext installs the process-wide context. A nil context is ignored.
func SetContext(ctx context.Context) {
	if ctx != nil {
		defaultContext = ctx
	}
}

// Context returns the process-wide context (never nil).
func Context() context.Context {
	if defaultContext == nil {
		return context.Background()
	}
	return defaultContext
}
