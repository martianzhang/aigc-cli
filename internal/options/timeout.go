package options

import (
	"time"

	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// ExplicitTimeout reports the user-configured timeout for modKey and whether
// one was set. Provider helpers use it to distinguish "no preference" (keep a
// generous built-in default such as 15 minutes for document parsing) from an
// explicit override.
func ExplicitTimeout(modKey string) (time.Duration, bool) {
	if Shared.TimeoutFlag > 0 {
		return time.Duration(Shared.TimeoutFlag) * time.Second, true
	}
	if seconds := ModalityTimeout(modKey); seconds != nil && *seconds > 0 {
		return time.Duration(*seconds) * time.Second, true
	}
	if cfg := currentConfig(); cfg != nil && cfg.Timeout != nil && *cfg.Timeout > 0 {
		return time.Duration(*cfg.Timeout) * time.Second, true
	}
	return 0, false
}

// ModalityTimeout returns defaults.{modKey}.timeout in seconds, or nil. It does
// not consult the global `timeout` key, which bounds one HTTP request rather
// than a multi-minute generation job.
func ModalityTimeout(modKey string) *int {
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
