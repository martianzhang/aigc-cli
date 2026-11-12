package clientfactory

import (
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// NewClient creates an API client from the resolved provider for cmdName.
// The client is bound to the process-wide context so Ctrl-C cancels requests.
func NewClient(cmdName string) *client.Client {
	p := options.Shared.ResolveProvider(cmdName)
	return client.NewFromProvider(p).WithContext(options.Context())
}

// ApplyTimeout sets the HTTP client timeout from CLI flag / config, falling
// back to modDefault. Priority: --timeout flag > defaults.{mod}.timeout >
// timeout > modDefault.
func ApplyTimeout(c client.APIClient, modKey string, modDefault time.Duration) {
	c.SetTimeout(resolveTimeout(modKey, modDefault))
}

// TaskWait returns how long to wait for an async task of the given media type
// (types.MediaImage / MediaVideo / MediaMusic) before giving up.
//
// Async polling reuses the same per-media knob as the HTTP timeout, with a
// per-media built-in fallback because generators queue very differently:
// Priority: --timeout flag > defaults.{media}.timeout > built-in default
// (image 10m, video 30m, music 15m).
//
// The global `timeout` key is intentionally not consulted here: it exists to
// bound a single HTTP request, not a multi-minute generation job.
func TaskWait(media string) time.Duration {
	fallback := types.DefaultTaskWait(media)
	if options.Shared.TimeoutFlag > 0 {
		return time.Duration(options.Shared.TimeoutFlag) * time.Second
	}
	if seconds := options.ModalityTimeout(media); seconds != nil && *seconds > 0 {
		return time.Duration(*seconds) * time.Second
	}
	return fallback
}

// HTTPTimeout returns the effective HTTP request timeout for modKey, falling
// back to modDefault. It exposes the same priority chain as ApplyTimeout so
// provider helpers that bypass client.Client (ModelScope, Ollama, OCR/vision,
// SFX) can honor the global --timeout flag too.
func HTTPTimeout(modKey string, modDefault time.Duration) time.Duration {
	return resolveTimeout(modKey, modDefault)
}

// resolveTimeout implements the priority chain of ApplyTimeout.
func resolveTimeout(modKey string, modDefault time.Duration) time.Duration {
	if timeout, ok := options.ExplicitTimeout(modKey); ok {
		return timeout
	}
	return modDefault
}
