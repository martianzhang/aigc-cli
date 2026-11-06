package options

import (
	"sort"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// DefaultsOrNil returns the top-level defaults section of the config, or nil.
func DefaultsOrNil() *types.ConfigDefaults {
	return Field(Shared.Cfg, func(c *types.Config) *types.ConfigDefaults { return c.Defaults })
}

// ChatDefaults returns defaults.chat, or nil.
func ChatDefaults() *types.ChatDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.ChatDefaults { return d.Chat })
}

// ChatBashTimeout returns the effective timeout for the chat bash tool and the
// TUI `!command` shortcut. defaults.chat.bash.timeout (seconds) wins; otherwise
// the built-in default applies.
func ChatBashTimeout() time.Duration {
	// Use currentConfig (not ChatDefaults) so the accessor also works before
	// PersistentPreRunE populated Shared.Cfg (tests, direct library use).
	cfg := currentConfig()
	var defaults *types.ConfigDefaults
	if cfg != nil {
		defaults = cfg.Defaults
	}
	chat := Field(defaults, func(d *types.ConfigDefaults) *types.ChatDefaults { return d.Chat })
	bash := Field(chat, func(c *types.ChatDefaults) *types.ChatBashDefaults { return c.Bash })
	if bash != nil && bash.Timeout > 0 {
		return time.Duration(bash.Timeout) * time.Second
	}
	return time.Duration(types.DefaultBashTimeoutSeconds) * time.Second
}

// NamedProvider returns providers.<name>, or nil when it is not configured.
func NamedProvider(name string) *types.NamedProvider {
	if name == "" || Shared.Cfg == nil {
		return nil
	}
	return Shared.Cfg.Providers[name]
}

// ProviderNames returns the configured named provider keys in stable order.
func ProviderNames() []string {
	if Shared.Cfg == nil || len(Shared.Cfg.Providers) == 0 {
		return nil
	}
	names := make([]string, 0, len(Shared.Cfg.Providers))
	for name := range Shared.Cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ImageDefaults returns defaults.image, or nil.
func ImageDefaults() *types.ImageDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.ImageDefaults { return d.Image })
}

// DecisionDefaults returns defaults.decision, or nil.
func DecisionDefaults() *types.DecisionDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.DecisionDefaults { return d.Decision })
}

// UpscaleDefaults returns defaults.upscale, or nil.
func UpscaleDefaults() *types.UpscaleDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.UpscaleDefaults { return d.Upscale })
}

// VideoDefaults returns defaults.video, or nil.
func VideoDefaults() *types.VideoDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.VideoDefaults { return d.Video })
}

// AudioDefaults returns defaults.audio, or nil.
func AudioDefaults() *types.AudioDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.AudioDefaults { return d.Audio })
}

// KnowledgeDefaults returns defaults.knowledgebase, or nil.
func KnowledgeDefaults() *types.KBDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.KBDefaults { return d.Knowledgebase })
}

// SearchDefaults returns defaults.search, or nil.
func SearchDefaults() *types.SearchDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.SearchDefaults { return d.Search })
}

// SearchProviderFromConfig returns defaults.search.provider, or "".
func SearchProviderFromConfig(cfg *types.Config) string {
	if cfg == nil || cfg.Defaults == nil || cfg.Defaults.Search == nil {
		return ""
	}
	return cfg.Defaults.Search.Provider
}

// SearchProviderDefault is SearchProviderFromConfig applied to the loaded config.
func SearchProviderDefault() string {
	return SearchProviderFromConfig(Shared.Cfg)
}

// IdeasDefaults returns defaults.ideas, or nil.
func IdeasDefaults() *types.IdeasConfig {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.IdeasConfig { return d.Ideas })
}

// MidjourneyDefaults returns defaults.midjourney, or nil.
func MidjourneyDefaults() *types.MidjourneyDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.MidjourneyDefaults { return d.Midjourney })
}

// MusicDefaults returns defaults.music, or nil.
func MusicDefaults() *types.MusicDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.MusicDefaults { return d.Music })
}

// DetectConfig returns the top-level detect section, or nil.
func DetectConfig() *types.DetectConfig {
	return Field(Shared.Cfg, func(c *types.Config) *types.DetectConfig { return c.Detect })
}

// BackgroundConfig returns the top-level background section, or nil.
func BackgroundConfig() *types.BackgroundConfig {
	return Field(Shared.Cfg, func(c *types.Config) *types.BackgroundConfig { return c.Background })
}

// SkillsConfig returns the top-level skills section, or nil.
func SkillsConfig() *types.SkillsConfig {
	return Field(Shared.Cfg, func(c *types.Config) *types.SkillsConfig { return c.Skills })
}

// WebSearchConfig returns the top-level web_search section, or nil.
func WebSearchConfig() map[string]*types.WebSearchProvider {
	return Field(Shared.Cfg, func(c *types.Config) map[string]*types.WebSearchProvider { return c.WebSearch })
}

// MCPConfig returns the top-level mcp section, or nil.
func MCPConfig() *types.MCPConfig {
	return Field(Shared.Cfg, func(c *types.Config) *types.MCPConfig { return c.MCP })
}
