package options

import (
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// NewClient creates an API client from the resolved provider for cmdName.
// The client is bound to the process-wide context so Ctrl-C cancels requests.
func NewClient(cmdName string) *client.Client {
	p := Shared.ResolveProvider(cmdName)
	return client.NewFromProvider(p).WithContext(Context())
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
	if Shared.TimeoutFlag > 0 {
		return time.Duration(Shared.TimeoutFlag) * time.Second
	}
	if seconds := modalityTimeout(media); seconds != nil && *seconds > 0 {
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

// ExplicitTimeout reports the user-configured timeout for modKey and whether
// one was set. Provider helpers use it to distinguish "no preference" (keep a
// generous built-in default such as 15 minutes for document parsing) from an
// explicit override.
func ExplicitTimeout(modKey string) (time.Duration, bool) {
	if Shared.TimeoutFlag > 0 {
		return time.Duration(Shared.TimeoutFlag) * time.Second, true
	}
	if seconds := modalityTimeout(modKey); seconds != nil && *seconds > 0 {
		return time.Duration(*seconds) * time.Second, true
	}
	if cfg := currentConfig(); cfg != nil && cfg.Timeout != nil && *cfg.Timeout > 0 {
		return time.Duration(*cfg.Timeout) * time.Second, true
	}
	return 0, false
}

// resolveTimeout implements the priority chain of ApplyTimeout.
func resolveTimeout(modKey string, modDefault time.Duration) time.Duration {
	if timeout, ok := ExplicitTimeout(modKey); ok {
		return timeout
	}
	return modDefault
}

// currentConfig returns the already-loaded config, falling back to reading the
// config file when Shared.Cfg is nil (tests, direct library use).
func currentConfig() *types.Config {
	if Shared.Cfg != nil {
		return Shared.Cfg
	}
	cfg, err := config.LoadDefaults(Shared.CfgFile)
	if err != nil {
		return nil
	}
	return cfg
}

// modalityTimeout returns defaults.{modKey}.timeout in seconds, or nil.
func modalityTimeout(modKey string) *int {
	cfg := currentConfig()
	if cfg == nil || cfg.Defaults == nil {
		return nil
	}
	d := cfg.Defaults
	switch modKey {
	case "image":
		if d.Image != nil {
			return d.Image.Timeout
		}
	case "video":
		if d.Video != nil {
			return d.Video.Timeout
		}
	case "midjourney":
		if d.Midjourney != nil {
			return d.Midjourney.Timeout
		}
	case "music":
		if d.Music != nil {
			return d.Music.Timeout
		}
	case "audio":
		if d.Audio != nil {
			return d.Audio.Timeout
		}
	}
	return nil
}
