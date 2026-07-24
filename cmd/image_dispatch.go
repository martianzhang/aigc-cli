package cmd

import (
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// imageDispatchCtx holds provider/mode context for image strategy matching.
// Built from local variables in runImageGenerate, not global state.
type imageDispatchCtx struct {
	isAPIMart    bool
	isOpenRouter bool
	genEdit      bool
	isOllama     bool
}

// imageStrategy defines a dispatch rule for image generation.
type imageStrategy struct {
	match func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool
	run   func(client.APIClient, *types.GenerateRequest, *imageDispatchCtx) error
}

// imageStrategies is the ordered dispatch table for image generation.
// First match wins. Add a new entry here when adding a new provider or model type.
var imageStrategies = []imageStrategy{
	{
		// OpenRouter: all image models -> Unified Image API (POST /v1/images)
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return ctx.isOpenRouter && !ctx.genEdit
		},
		run: runOpenRouterDedicatedImage,
	},
	{
		// APIMart: async task-based generation
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return ctx.isAPIMart
		},
		run: runAsyncImage,
	},
	{
		// Ollama/local: native /api/generate endpoint (not OpenAI-compatible)
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return ctx.isOllama
		},
		run: runOllamaImage,
	},
	// Default: OpenAI-compatible synchronous generation
	{
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool { return true },
		run:   runSyncImage,
	},
}
