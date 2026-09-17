package image

import (
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// imageDispatchCtx holds provider/mode context for image strategy matching.
// Built from local variables in runImageGenerate, not global state.
type imageDispatchCtx struct {
	isAPIMart     bool
	isOpenRouter  bool
	isModelScope  bool
	isAgnes       bool
	isGemini      bool
	isZeekai      bool
	genEdit       bool
	isOllama      bool
	modelScopeKey string // API key for ModelScope async submission
}

// zeekaiImageEdits is the single definition of when a detected ZeekAI provider
// must use the edits-JSON protocol: only for image-to-image, so text-only
// generation keeps falling through to the normal sync path.
func zeekaiImageEdits(isZeekai bool, imageCount int) bool {
	return isZeekai && imageCount > 0
}

// usesImageEditsJSON reports whether p+req must route through POST /images/edits
// with an images[].image_url body: an auto-detected ZeekAI provider handling
// image input. Shared by the CLI strategy table and the chat/agent
// GenerateAndSave path.
func usesImageEditsJSON(p *provider.EffectiveProvider, req *types.GenerateRequest) bool {
	return p != nil && req != nil && zeekaiImageEdits(p.ProviderType == provider.Zeekai, len(req.ImageURLs))
}

// imageStrategy defines a dispatch rule for image generation.
// run returns the paths to saved image files, or an error.
type imageStrategy struct {
	match func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool
	run   func(client.APIClient, *types.GenerateRequest, *imageDispatchCtx) ([]string, error)
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
		// Agnes: dedicated sync API with extra_body.image for img2img and ratio for 2.1
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return ctx.isAgnes
		},
		run: runAgnesImage,
	},
	{
		// Gemini: native generateContent API with image output
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return ctx.isGemini
		},
		run: runGeminiImage,
	},
	{
		// ModelScope: async task-based generation (X-ModelScope-Async-Mode + polling)
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return ctx.isModelScope
		},
		run: runModelScopeImage,
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
	{
		// ZeekAI: image-to-image auto-routes to POST /images/edits with
		// images[].image_url; text-only generation falls through to sync.
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return zeekaiImageEdits(ctx.isZeekai, len(req.ImageURLs))
		},
		run: runImageEditsJSON,
	},
	// Default: OpenAI-compatible synchronous generation
	{
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool { return true },
		run:   runSyncImage,
	},
}
