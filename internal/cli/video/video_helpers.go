package video

import (
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// loadVideoDefaults returns the user's video config defaults.
// Tries options.Shared.Cfg first (fast), falls back to reading from file.
func loadVideoDefaults() *types.VideoDefaults {
	if cfg := options.VideoDefaults(); cfg != nil {
		return cfg
	}
	if cfg, err := config.Load(options.Shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
		return cfg.Defaults.Video
	}
	return nil
}

// GenerateAndSave generates videos via the configured provider and saves them to disk.
// Handles config merge and API dispatch. Returns paths to saved files.
// Shared by CLI (video command) and agent loop (chat) — single source of truth.
// Supports APIMart async and OpenRouter video providers.
func GenerateAndSave(req *types.VideoGenerateRequest) ([]string, error) {
	// Always load the user's config — options.Shared.Cfg may be nil if PersistentPreRunE hasn't run.
	vidCfg := loadVideoDefaults()

	// Check if LLM is allowed to override (default: false = config wins)
	allowOverride := false
	if cfg := options.ChatDefaults(); cfg != nil {
		allowOverride = cfg.AllowToolOverride
	}

	if vidCfg != nil {
		if !allowOverride {
			if vidCfg.Model != "" {
				req.Model = vidCfg.Model
			}
			if vidCfg.Size != "" {
				req.Size = vidCfg.Size
			}
			if vidCfg.Resolution != "" {
				req.Resolution = vidCfg.Resolution
			}
			if vidCfg.Duration != nil {
				req.Duration = vidCfg.Duration
			}
		} else {
			vidCfg.MergeIntoVideo(req)
		}
	}
	// Code defaults for fields the user didn't configure
	if req.Size == "" {
		req.Size = "16:9"
	}
	if req.Resolution == "" {
		req.Resolution = "480p"
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required: set via defaults.video.model in config.yaml")
	}

	// Resolve provider (named provider > global > builtin) and dispatch.
	// Each strategy runner builds its own video-scoped client internally.
	p := options.Shared.ResolveProvider(options.ProviderNameVideo)
	plan, err := buildVideoPlan(req, p)
	if err != nil {
		return nil, err
	}
	if len(plan.Uploads) > 0 {
		c := options.NewClient(options.ProviderNameVideo)
		if err := plan.applyUploads(c, req); err != nil {
			return nil, err
		}
	}
	vctx := &videoDispatchCtx{
		isOpenRouter:   p.ProviderType == provider.OpenRouter,
		isOpenLux:      p.ProviderType == provider.OpenLux,
		isAgnes:        p.ProviderType == provider.Agnes,
		isPollinations: p.ProviderType == provider.Pollinations,
	}
	for _, s := range videoStrategies {
		if s.match(req, vctx) {
			return s.run(req)
		}
	}
	return nil, fmt.Errorf("no video strategy matched")
}
