package options

import (
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
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
	d := modDefault
	if Shared.TimeoutFlag > 0 {
		d = time.Duration(Shared.TimeoutFlag) * time.Second
		c.SetTimeout(d)
		return
	}
	if cfg, err := config.LoadDefaults(Shared.CfgFile); err == nil && cfg != nil {
		var modTimeout *int
		if cfg.Defaults != nil {
			switch modKey {
			case "image":
				if cfg.Defaults.Image != nil {
					modTimeout = cfg.Defaults.Image.Timeout
				}
			case "video":
				if cfg.Defaults.Video != nil {
					modTimeout = cfg.Defaults.Video.Timeout
				}
			case "midjourney":
				if cfg.Defaults.Midjourney != nil {
					modTimeout = cfg.Defaults.Midjourney.Timeout
				}
			case "music":
				if cfg.Defaults.Music != nil {
					modTimeout = cfg.Defaults.Music.Timeout
				}
			case "audio":
				if cfg.Defaults.Audio != nil {
					modTimeout = cfg.Defaults.Audio.Timeout
				}
			}
		}
		if modTimeout != nil && *modTimeout > 0 {
			d = time.Duration(*modTimeout) * time.Second
		} else if cfg.Timeout != nil && *cfg.Timeout > 0 {
			d = time.Duration(*cfg.Timeout) * time.Second
		}
	}
	c.SetTimeout(d)
}
