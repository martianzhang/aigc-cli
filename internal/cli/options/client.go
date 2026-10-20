package options

import (
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// NewClient creates an API client from the resolved provider for cmdName.
func NewClient(cmdName string) *client.Client {
	p := Shared.ResolveProvider(cmdName)
	return client.NewFromProvider(p)
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

// resolveTimeout implements the priority chain of ApplyTimeout.
func resolveTimeout(modKey string, modDefault time.Duration) time.Duration {
	if Shared.TimeoutFlag > 0 {
		return time.Duration(Shared.TimeoutFlag) * time.Second
	}
	if seconds := modalityTimeout(modKey); seconds != nil && *seconds > 0 {
		return time.Duration(*seconds) * time.Second
	}
	if cfg, err := config.LoadDefaults(Shared.CfgFile); err == nil && cfg != nil {
		if cfg.Timeout != nil && *cfg.Timeout > 0 {
			return time.Duration(*cfg.Timeout) * time.Second
		}
	}
	return modDefault
}

// modalityTimeout returns defaults.{modKey}.timeout in seconds, or nil.
func modalityTimeout(modKey string) *int {
	cfg, err := config.LoadDefaults(Shared.CfgFile)
	if err != nil || cfg == nil || cfg.Defaults == nil {
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
