package options

import "github.com/martianzhang/aigc-cli/internal/types"

// DefaultsOrNil returns the top-level defaults section of the config, or nil.
func DefaultsOrNil() *types.ConfigDefaults {
	return Field(Shared.Cfg, func(c *types.Config) *types.ConfigDefaults { return c.Defaults })
}

// ChatDefaults returns defaults.chat, or nil.
func ChatDefaults() *types.ChatDefaults {
	return Field(DefaultsOrNil(), func(d *types.ConfigDefaults) *types.ChatDefaults { return d.Chat })
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

// WebSearchConfig returns the top-level web_search section, or nil.
func WebSearchConfig() map[string]*types.WebSearchProvider {
	return Field(Shared.Cfg, func(c *types.Config) map[string]*types.WebSearchProvider { return c.WebSearch })
}

// GlobalSearchProvider returns the top-level search_provider default (the
// global fallback vendor for the third-party search tools), or "".
func GlobalSearchProvider() string {
	return Field(Shared.Cfg, func(c *types.Config) string { return c.SearchProvider })
}

// MCPConfig returns the top-level mcp section, or nil.
func MCPConfig() *types.MCPConfig {
	return Field(Shared.Cfg, func(c *types.Config) *types.MCPConfig { return c.MCP })
}
