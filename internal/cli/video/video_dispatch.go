package video

import (
	"github.com/martianzhang/aigc-cli/internal/types"
)

// videoDispatchCtx holds provider context for video strategy matching.
// Built from local variables in runVideo, not global state.
type videoDispatchCtx struct {
	isOpenRouter   bool
	isOpenLux      bool
	isAgnes        bool
	isPollinations bool
}

// videoStrategy defines a dispatch rule for video generation.
// run returns the paths to saved video files, or an error.
type videoStrategy struct {
	match func(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) bool
	run   func(*types.VideoGenerateRequest) ([]string, error)
}

// videoStrategies is the ordered dispatch table for video generation.
// First match wins. Add a new entry here when adding a new provider.
var videoStrategies = []videoStrategy{
	{
		// OpenRouter: dedicated video API (submit -> poll -> download)
		match: func(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) bool {
			return ctx.isOpenRouter
		},
		run: runOpenRouterVideo,
	},
	{
		// Agnes: async task API (submit -> poll -> download)
		match: func(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) bool {
			return ctx.isAgnes
		},
		run: runAgnesVideo,
	},
	{
		// OpenLux: unified video API (submit -> poll -> download)
		match: func(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) bool {
			return ctx.isOpenLux
		},
		run: runOpenLuxVideo,
	},
	{
		// Pollinations: synchronous GET /video/{prompt} returning raw MP4 bytes
		match: func(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) bool {
			return ctx.isPollinations
		},
		run: runPollinationsVideo,
	},
	{
		// Default: APIMart async task-based generation
		match: func(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) bool { return true },
		run:   runAPIMartVideo,
	},
}
