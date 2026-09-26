// Package options holds the shared runtime configuration and config
// accessors used by CLI command packages. Keeping them here lets command
// subpackages read config/state without importing the `cmd` package.
package options

import (
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// SharedConfig holds all shared configuration values that were previously
// individual global variables. Initialized in PersistentPreRunE.
type SharedConfig struct {
	CfgFile     string
	APIKey      string
	APIBase     string
	HTTPProxy   string
	Model       string
	Provider    string // --provider flag: override named provider for all commands
	JSONInput   string
	OutputDir   string
	Verbose     bool
	SavePrompt  bool
	Mode        string
	PrintConfig bool
	TimeoutFlag int
	ZDR         bool
	Cfg         *types.Config // full parsed config (may be nil)

	APIKeySet   bool
	APIBaseSet  bool
	ProviderSet bool
	ZDRSet      bool
}

// Shared is the process-wide shared configuration instance.
var Shared = &SharedConfig{}

// Provider name constants for ResolveProvider.
const (
	ProviderNameImage      = "image"
	ProviderNameVideo      = "video"
	ProviderNameChat       = "chat"
	ProviderNameAudio      = "audio"
	ProviderNameMidjourney = "midjourney"
	ProviderNameMusic      = "music"
	ProviderNameModels     = "models"
	ProviderNameOCR        = "ocr"
	ProviderNameVision     = "vision"
	ProviderNameDetect     = "detect"
	ProviderNameBackground = "background"
	ProviderNameLocal      = "local"
)

// ResolveProvider returns the effective provider configuration for a command.
// Priority (highest to lowest):
//  1. CLI flags (--api-key / --api-base) — if set, skip all config
//  2. defaults.{cmd}.provider → providers.{name}
//  3. Global config (top-level api_key / base_url)
//  4. Built-in defaults (APIMart)
func (s *SharedConfig) ResolveProvider(cmdName string) *provider.EffectiveProvider {
	var cli *provider.CLIOverride
	if s.APIKeySet || s.APIBaseSet {
		cli = &provider.CLIOverride{
			APIKey:  s.APIKey,
			BaseURL: s.APIBase,
			Proxy:   s.HTTPProxy,
			Model:   s.Model,
		}
	}

	var providerRef, defaultsModel string
	if s.ProviderSet && s.Provider != "" {
		providerRef = s.Provider
	} else if s.Cfg != nil && s.Cfg.Defaults != nil {
		providerRef, defaultsModel = LookupCmdProviderAndModel(cmdName, s.Cfg.Defaults)
	}

	if providerRef == ProviderNameLocal {
		return &provider.EffectiveProvider{
			Name:  ProviderNameLocal,
			Type:  types.ProviderLocal,
			Model: defaultsModel,
		}
	}

	global := &provider.GlobalConfig{
		APIKey:  cfgString(s.Cfg, func(c *types.Config) string { return c.APIKey }),
		BaseURL: cfgString(s.Cfg, func(c *types.Config) string { return c.BaseURL }),
		Proxy:   cfgString(s.Cfg, func(c *types.Config) string { return c.HTTPProxy }),
	}

	ep := provider.ResolveCmdProvider(cli, providerRef, providerMap(s.Cfg), global)
	ep.ZDR = s.resolveZDR(ep)
	if s.Model != "" {
		ep.Model = s.Model
	} else if defaultsModel != "" {
		ep.Model = defaultsModel
	}
	return ep
}

// CmdProviderInfo describes how to extract provider/model for a single command
// from the ConfigDefaults struct.
type CmdProviderInfo struct {
	Getter func(d *types.ConfigDefaults) (provider string, model string)
}

// CmdProviderMap is the single source of truth for which fields to query for
// each command's defaults.{cmd}.provider and defaults.{cmd}.model.
var CmdProviderMap = map[string]CmdProviderInfo{
	ProviderNameImage: {func(d *types.ConfigDefaults) (string, string) {
		if d.Image != nil {
			return d.Image.Provider, d.Image.Model
		}
		return "", ""
	}},
	ProviderNameVideo: {func(d *types.ConfigDefaults) (string, string) {
		if d.Video != nil {
			return d.Video.Provider, d.Video.Model
		}
		return "", ""
	}},
	ProviderNameChat: {func(d *types.ConfigDefaults) (string, string) {
		if d.Chat != nil {
			return d.Chat.Provider, d.Chat.Model
		}
		return "", ""
	}},
	ProviderNameAudio: {func(d *types.ConfigDefaults) (string, string) {
		if d.Audio != nil {
			return d.Audio.Provider, ""
		}
		return "", ""
	}},
	ProviderNameMidjourney: {func(d *types.ConfigDefaults) (string, string) {
		if d.Midjourney != nil {
			return d.Midjourney.Provider, ""
		}
		return "", ""
	}},
	ProviderNameMusic: {func(d *types.ConfigDefaults) (string, string) {
		if d.Music != nil {
			return d.Music.Provider, d.Music.Model
		}
		return "", ""
	}},
	ProviderNameOCR: {func(d *types.ConfigDefaults) (string, string) {
		if d.OCR != nil {
			return d.OCR.Provider, d.OCR.Model
		}
		return "", ""
	}},
	ProviderNameVision: {func(d *types.ConfigDefaults) (string, string) {
		if d.Vision != nil {
			return d.Vision.Provider, d.Vision.Model
		}
		return "", ""
	}},
}

// LookupCmdProviderAndModel returns the provider reference and model for a command.
func LookupCmdProviderAndModel(cmdName string, d *types.ConfigDefaults) (provider string, model string) {
	if d == nil {
		return "", ""
	}
	if info, ok := CmdProviderMap[cmdName]; ok {
		return info.Getter(d)
	}
	return "", ""
}

func cfgString(cfg *types.Config, getter func(*types.Config) string) string {
	if cfg == nil {
		return ""
	}
	return getter(cfg)
}

func providerMap(cfg *types.Config) map[string]*types.NamedProvider {
	if cfg == nil {
		return nil
	}
	return cfg.Providers
}

// resolveZDR applies ZDR precedence: an explicit --zdr flag wins, then a
// providers.{name}.zdr override, then the global config zdr, then false.
func (s *SharedConfig) resolveZDR(ep *provider.EffectiveProvider) bool {
	if s.ZDRSet {
		return s.ZDR
	}
	if v, ok := providerZDR(s.Cfg, ep.Name); ok {
		return v
	}
	return s.Cfg != nil && s.Cfg.ZDR
}

// providerZDR returns the per-provider zdr override and whether it is set.
func providerZDR(cfg *types.Config, name string) (value, set bool) {
	if cfg == nil || name == "" {
		return false, false
	}
	np, ok := cfg.Providers[name]
	if !ok || np == nil || np.ZDR == nil {
		return false, false
	}
	return *np.ZDR, true
}
