package cmd

import (
	"io"
	"path"

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
	Cfg         *types.Config // full parsed config (may be nil)

	// APIKeySet and APIBaseSet are true when the user explicitly passed
	// --api-key / --api-base on the command line (vs inherited from config).
	// Used by ResolveProvider to decide whether to override named providers.
	APIKeySet   bool
	APIBaseSet  bool
	ProviderSet bool // true when --provider was explicitly set
}

// Provider name constants for ResolveProvider. Using constants instead of
// string literals prevents typos and makes refactoring safer.
const (
	ProviderNameImage      = "image"
	ProviderNameVideo      = "video"
	ProviderNameChat       = "chat"
	ProviderNameAudio      = "audio"
	ProviderNameMidjourney = "midjourney"
	ProviderNameModels     = "models"
	ProviderNameOCR        = "ocr"
	ProviderNameVision     = "vision"
	ProviderNameDetect     = "detect"
	ProviderNameBackground = "background"
	// ProviderNameLocal is a built-in pseudo-provider that routes to local
	// ONNX inference (audio, ocr, vision, detect, background).
	// No config definition needed — handled specially in ResolveProvider.
	ProviderNameLocal = "local"
)

// defaultBaseURL is the built-in fallback when no base_url is configured anywhere.
const defaultBaseURL = "https://api.apimart.ai"

// ResolveProvider returns the effective provider configuration for a command.
// Priority (highest to lowest):
//  1. CLI flags (--api-key / --api-base) — if set, skip all config
//  2. defaults.{cmd}.provider → providers.{name}
//  3. Global config (top-level api_key / base_url)
//  4. Built-in defaults (APIMart)
func (s *SharedConfig) ResolveProvider(cmdName string) *provider.EffectiveProvider {
	// Build CLI override: only set values when the user explicitly passed
	// --api-key / --api-base on the command line (not inherited from config).
	var cli *provider.CLIOverride
	if s.APIKeySet || s.APIBaseSet {
		cli = &provider.CLIOverride{
			APIKey:  s.APIKey,
			BaseURL: s.APIBase,
			Proxy:   s.HTTPProxy,
			Model:   s.Model,
		}
	}

	// Extract provider reference: --provider flag takes priority over defaults
	var providerRef, defaultsModel string
	if s.ProviderSet && s.Provider != "" {
		providerRef = s.Provider
	} else if s.Cfg != nil && s.Cfg.Defaults != nil {
		providerRef, defaultsModel = lookupCmdProviderAndModel(cmdName, s.Cfg.Defaults)
	}

	// Built-in "local" provider — routes to local ONNX inference.
	// No config definition needed. Overrides all other resolution.
	if providerRef == ProviderNameLocal {
		return &provider.EffectiveProvider{
			Name:  ProviderNameLocal,
			Type:  types.ProviderLocal,
			Model: defaultsModel,
		}
	}

	// Build global config
	global := &provider.GlobalConfig{
		APIKey:  cfgString(s.Cfg, func(c *types.Config) string { return c.APIKey }),
		BaseURL: cfgString(s.Cfg, func(c *types.Config) string { return c.BaseURL }),
		Proxy:   cfgString(s.Cfg, func(c *types.Config) string { return c.HTTPProxy }),
	}

	ep := provider.ResolveCmdProvider(cli, providerRef, providerMap(s.Cfg), global)
	// Ensure BaseURL is never empty
	if ep.BaseURL == "" {
		ep.BaseURL = defaultBaseURL
	}
	// Model priority: CLI --model > defaults.{cmd}.model > providers.{name}.model
	if s.Model != "" {
		ep.Model = s.Model
	} else if defaultsModel != "" {
		ep.Model = defaultsModel
	}
	return ep
}

// lookupCmdProviderAndModel returns the provider reference and model for a command.
func lookupCmdProviderAndModel(cmdName string, d *types.ConfigDefaults) (provider string, model string) {
	if d == nil {
		return "", ""
	}
	switch cmdName {
	case "image":
		if d.Image != nil {
			return d.Image.Provider, d.Image.Model
		}
	case "video":
		if d.Video != nil {
			return d.Video.Provider, d.Video.Model
		}
	case "chat":
		if d.Chat != nil {
			return d.Chat.Provider, d.Chat.Model
		}
	case "audio":
		if d.Audio != nil {
			return d.Audio.Provider, ""
		}
	case "midjourney":
		if d.Midjourney != nil {
			return d.Midjourney.Provider, ""
		}
	case "ocr":
		if d.OCR != nil {
			return d.OCR.Provider, d.OCR.Model
		}
	case "vision":
		if d.Vision != nil {
			return d.Vision.Provider, d.Vision.Model
		}
	}
	return "", ""
}

// cfgString returns the string from config via getter, or empty string if cfg is nil.
func cfgString(cfg *types.Config, getter func(*types.Config) string) string {
	if cfg == nil {
		return ""
	}
	return getter(cfg)
}

// providerMap returns the providers map from config, or nil.
func providerMap(cfg *types.Config) map[string]*types.NamedProvider {
	if cfg == nil {
		return nil
	}
	return cfg.Providers
}

// SetSharedForTest sets the global shared config to a test-specific value and
// returns a cleanup function that restores the previous state. Tests should
// defer the cleanup:
//
//	defer SetSharedForTest(&SharedConfig{APIKey: "test", ...})()
func SetSharedForTest(sc *SharedConfig) func() {
	old := *shared
	*shared = *sc
	return func() { *shared = old }
}

// SetChatOutputForTest overrides the chat REPL output writers for testing and
// returns a cleanup function. Useful for capturing chat output in tests:
//
//	var stdout, stderr strings.Builder
//	defer SetChatOutputForTest(&stdout, &stderr)()
//
// matchAny returns true if name matches any of the glob patterns.
func matchAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if matched, _ := path.Match(p, name); matched {
			return true
		}
	}
	return false
}

// isToolAllowed checks if a tool is allowed by global tools_enable/tools_disable rules.
// Empty enable list = all tools allowed. When enable is set, tool must match at least one pattern.
// disable is a blacklist applied on top.
func isToolAllowed(toolName string, enable, disable []string) bool {
	if len(enable) > 0 && !matchAny(toolName, enable) {
		return false
	}
	if matchAny(toolName, disable) {
		return false
	}
	return true
}

func SetChatOutputForTest(stdout, stderr io.Writer) func() {
	oldStdout, oldStderr := chatStdout, chatStderr
	chatStdout, chatStderr = stdout, stderr
	return func() { chatStdout, chatStderr = oldStdout, oldStderr }
}
