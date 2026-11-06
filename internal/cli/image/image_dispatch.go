package image

import (
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// imageDispatchCtx holds provider/mode context for image strategy matching.
// Built from the resolved provider, not global state, so a per-call provider
// (MCP `provider` argument) drives both the strategy and provider-specific
// request details such as the ModelScope API key.
type imageDispatchCtx struct {
	// p is the provider that owns this request. Runners use it for
	// provider-specific auth and user-facing labels. It is nil only in unit
	// tests that exercise strategy matching without a provider.
	p *provider.EffectiveProvider

	isAPIMart     bool
	isOpenRouter  bool
	isModelScope  bool
	isAgnes       bool
	isGemini      bool
	isZeekai      bool
	isMultipart   bool // OpenAI-family /images/edits (multipart/form-data)
	genEdit       bool
	isOllama      bool
	modelScopeKey string // API key for ModelScope async submission
}

// newImageDispatchCtx derives the strategy flags from the resolved provider.
// Every caller that executes a strategy must build its context here, so the
// per-call provider cannot silently diverge from the client and API key used
// for the request.
func newImageDispatchCtx(p *provider.EffectiveProvider, req *types.GenerateRequest, genEdit bool) *imageDispatchCtx {
	if p == nil {
		p = &provider.EffectiveProvider{}
	}
	return &imageDispatchCtx{
		p:             p,
		isAPIMart:     options.IsAPIMartProvider(p),
		isOpenRouter:  p.ProviderType == provider.OpenRouter,
		isAgnes:       p.ProviderType == provider.Agnes,
		isOllama:      p.Type == types.ProviderOllama || provider.IsLocalEndpoint(p.BaseURL),
		isModelScope:  p.ProviderType == provider.ModelScope,
		isGemini:      p.ProviderType == provider.Gemini,
		isZeekai:      p.ProviderType == provider.Zeekai,
		isMultipart:   useMultipartImageEdits(p, req),
		genEdit:       genEdit,
		modelScopeKey: p.APIKey,
	}
}

// providerFor returns the provider that owns this dispatch. Tests may build a
// bare context, so fall back to the current CLI config instead of panicking.
func (ctx *imageDispatchCtx) providerFor() *provider.EffectiveProvider {
	if ctx != nil && ctx.p != nil {
		return ctx.p
	}
	return options.Shared.ResolveProvider(options.ProviderNameImage)
}

// providerLabel renders a provider name for logs and errors. Named providers
// keep their configured name; un-named providers use the detected type so the
// message never blames a different vendor than the one that received the
// request.
func providerLabel(p *provider.EffectiveProvider) string {
	if p == nil {
		return "image"
	}
	if p.Name != "" {
		return p.Name
	}
	if p.ProviderType != provider.Unknown {
		return strings.ToLower(p.ProviderType.String())
	}
	if p.Type != "" {
		return string(p.Type)
	}
	return "image"
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

// useMultipartImageEdits reports whether p+req must use the OpenAI-family
// multipart /images/edits protocol.
//
//   - --edit forces it for any OpenAI-compatible provider (manual override for
//     relays whose domain is not auto-detected), and also unlocks a --json body
//     by mapping its keys to form fields.
//   - Auto-detection applies only to OpenAI-family domains, and only when no
//     verbatim --json body is set: --json passthrough wins, so it keeps the
//     JSON /images/generations path.
func useMultipartImageEdits(p *provider.EffectiveProvider, req *types.GenerateRequest) bool {
	if p == nil || req == nil || len(req.ImageURLs) == 0 {
		return false
	}
	if genEdit {
		return p.ProviderType == provider.OpenAI || provider.NeedsMultipartImageEdits(p.BaseURL)
	}
	return len(req.RawJSON) == 0 && provider.NeedsMultipartImageEdits(p.BaseURL)
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
	{
		// OpenAI-family (native OpenAI, Azure, OfoxAI): reference images go to
		// POST /images/edits as multipart/form-data, because /images/generations
		// has no image field. Text-only generation falls through to sync.
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool {
			return ctx.isMultipart && len(req.ImageURLs) > 0
		},
		run: runImageEditsMultipart,
	},
	// Default: OpenAI-compatible synchronous generation
	{
		match: func(req *types.GenerateRequest, ctx *imageDispatchCtx) bool { return true },
		run:   runSyncImage,
	},
}
