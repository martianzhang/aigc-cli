package cmd

import (
	"github.com/martianzhang/apimart-cli/internal/types"
)

// videoDispatchCtx holds provider context for video strategy matching.
// Built from local variables in runVideo, not global state.
type videoDispatchCtx struct {
	isOpenRouter bool
	isYunwu      bool
}

// videoStrategy defines a dispatch rule for video generation.
type videoStrategy struct {
	match func(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) bool
	run   func(*types.VideoGenerateRequest) error
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
		// Yunwu (云雾AI): unified video API (submit -> poll -> download)
		match: func(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) bool {
			return ctx.isYunwu
		},
		run: runYunwuVideo,
	},
	{
		// Default: APIMart async task-based generation
		match: func(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) bool { return true },
		run:   runAPIMartVideo,
	},
}
