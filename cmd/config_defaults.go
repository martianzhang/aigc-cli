package cmd

import "github.com/martianzhang/aigc-cli/internal/types"

// defaultsOrNil returns the top-level defaults section of the config, or nil.
func defaultsOrNil() *types.ConfigDefaults {
	return field(shared.Cfg, func(c *types.Config) *types.ConfigDefaults { return c.Defaults })
}

// chatDefaults returns defaults.chat, or nil.
func chatDefaults() *types.ChatDefaults {
	return field(defaultsOrNil(), func(d *types.ConfigDefaults) *types.ChatDefaults { return d.Chat })
}

// imageDefaults returns defaults.image, or nil.
func imageDefaults() *types.ImageDefaults {
	return field(defaultsOrNil(), func(d *types.ConfigDefaults) *types.ImageDefaults { return d.Image })
}

// videoDefaults returns defaults.video, or nil.
func videoDefaults() *types.VideoDefaults {
	return field(defaultsOrNil(), func(d *types.ConfigDefaults) *types.VideoDefaults { return d.Video })
}

// audioDefaults returns defaults.audio, or nil.
func audioDefaults() *types.AudioDefaults {
	return field(defaultsOrNil(), func(d *types.ConfigDefaults) *types.AudioDefaults { return d.Audio })
}

// knowledgeDefaults returns defaults.knowledgebase, or nil.
func knowledgeDefaults() *types.KBDefaults {
	return field(defaultsOrNil(), func(d *types.ConfigDefaults) *types.KBDefaults { return d.Knowledgebase })
}

// midjourneyDefaults returns defaults.midjourney, or nil.
func midjourneyDefaults() *types.MidjourneyDefaults {
	return field(defaultsOrNil(), func(d *types.ConfigDefaults) *types.MidjourneyDefaults { return d.Midjourney })
}

// detectConfig returns the top-level detect section, or nil.
func detectConfig() *types.DetectConfig {
	return field(shared.Cfg, func(c *types.Config) *types.DetectConfig { return c.Detect })
}

// backgroundConfig returns the top-level background section, or nil.
func backgroundConfig() *types.BackgroundConfig {
	return field(shared.Cfg, func(c *types.Config) *types.BackgroundConfig { return c.Background })
}

// webSearchConfig returns the top-level web_search section, or nil.
func webSearchConfig() map[string]*types.WebSearchProvider {
	return field(shared.Cfg, func(c *types.Config) map[string]*types.WebSearchProvider { return c.WebSearch })
}
