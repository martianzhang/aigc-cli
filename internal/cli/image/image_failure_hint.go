package image

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// sizeErrorKeywords mark an API failure as (probably) size-related. Matching is
// case-insensitive. Kept narrow so unrelated 400s do not get noise appended:
// a bare "ratio" would match "generation", which appears in almost every
// wrapped error message.
var sizeErrorKeywords = []string{
	"size", "aspect", "resolution", "dimension",
	"width", "height", "像素", "尺寸", "宽高比",
}

// nonSizeErrorKeywords mark failures that are clearly NOT about size, so the
// generic "any 400 with a size set" fallback does not append a misleading
// --size hint. A content-safety / moderation rejection is the motivating case:
// it must never suggest the user change --size. Matching is case-insensitive.
var nonSizeErrorKeywords = []string{
	"content safety", "safety policy", "moderation", "content policy",
	"content_policy", "flagged", "内容安全", "安全策略", "违规",
}

// imageFailureHint returns an actionable, provider-specific --size hint for an
// image generation failure, or "" when the error does not look size-related.
// It is best-effort: capability lookups that fail degrade to a static hint.
func imageFailureHint(p *provider.EffectiveProvider, req *types.GenerateRequest, err error) string {
	if p == nil || err == nil || !looksLikeSizeError(err, req) {
		return ""
	}
	if p.ProviderType == provider.OpenRouter {
		return openRouterSizeHint(p, req)
	}
	return staticSizeHint(p)
}

// looksLikeSizeError reports whether err (and the request that produced it)
// points at a --size/--ratio/--resolution problem worth advising on.
func looksLikeSizeError(err error, req *types.GenerateRequest) bool {
	msg := strings.ToLower(err.Error())
	for _, kw := range nonSizeErrorKeywords {
		if strings.Contains(msg, kw) {
			return false
		}
	}
	for _, kw := range sizeErrorKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	if code, ok := client.StatusCode(err); ok && (code == http.StatusBadRequest || code == http.StatusUnprocessableEntity) {
		return req != nil && (req.Size != "" || req.Ratio != "" || req.Resolution != "")
	}
	return false
}

// openRouterSizeHint consults OpenRouter's public image model discovery
// (supported_parameters) and reports exactly which size fields the model
// accepts, or that it declares none at all.
func openRouterSizeHint(p *provider.EffectiveProvider, req *types.GenerateRequest) string {
	base := client.NormalizeBaseURL(p.BaseURL)
	params, found := client.OpenRouterImageParams(p.BaseURL, req.Model)

	var b strings.Builder
	b.WriteString("\n\n  --size hint (OpenRouter):")
	if !found {
		b.WriteString("\n    could not read this model's capabilities.")
		b.WriteString("\n    OpenRouter accepts an aspect ratio via --ratio (e.g. 16:9) or")
		b.WriteString("\n    a tier/pixels via --size (e.g. 2K, 1024x1024); --size 2K@16:9 sets both.")
	} else {
		aspects, hasAspect := enumParam(params, "aspect_ratio")
		tiers, hasTier := enumParam(params, "resolution")
		_, hasSize := params["size"]
		switch {
		case !hasAspect && !hasTier && !hasSize:
			fmt.Fprintf(&b, "\n    model %q declares no size parameter.", req.Model)
			b.WriteString("\n    --size / --ratio / --quality / --output_format are dropped")
			b.WriteString("\n    automatically before sending, so this error is not a size problem.")
		default:
			if hasAspect {
				fmt.Fprintf(&b, "\n    aspect ratio (--ratio): %s", strings.Join(aspects, " | "))
			}
			if hasTier {
				fmt.Fprintf(&b, "\n    resolution tier (--size or --resolution): %s", strings.Join(tiers, " | "))
			}
			if hasSize {
				b.WriteString("\n    size: any tier or WxH via --size (e.g. 2K, 1024x1024)")
			}
			// Only suggest combining the two fields when the model declares
			// both; otherwise the advice would name a parameter it rejects.
			switch {
			case hasAspect && hasTier:
				b.WriteString("\n    combine with --size <tier>@<ratio> (e.g. 2K@16:9).")
			case hasAspect:
				b.WriteString("\n    this model declares no resolution tier — pass the ratio alone, e.g. --size 16:9.")
			case hasTier:
				b.WriteString("\n    this model declares no aspect ratio — pass the tier alone, e.g. --size 2K.")
			}
		}
	}
	fmt.Fprintf(&b, "\n    inspect: aigc-cli models --api-base %s --type image", base)
	return b.String()
}

// enumParam returns a parameter's enum values when it is a non-empty enum, so
// the hint only advertises values the model actually accepts.
func enumParam(params map[string]types.OpenRouterParamDescriptor, name string) ([]string, bool) {
	d, ok := params[name]
	if !ok || d.Type != "enum" || len(d.Values) == 0 {
		return nil, false
	}
	return d.Values, true
}

// staticSizeHint emits the documented accepted --size forms for providers that
// expose no machine-readable capability list.
func staticSizeHint(p *provider.EffectiveProvider) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n  --size hint (%s):", p.ProviderType.String())
	fmt.Fprintf(&b, "\n    accepted: %s", staticSizeForms(p.ProviderType))
	b.WriteString("\n    see docs/*/guide-image.md")
	return b.String()
}

// staticSizeForms is the single source of the per-provider --size cheat sheet
// shown after a failure. Keep it in sync with docs/*/guide-image.md.
func staticSizeForms(t provider.Type) string {
	switch t {
	case provider.APIMart:
		return "--size 16:9 or 1024x1024; resolution tiers use --resolution 1k/2k/4k"
	case provider.Agnes:
		return "image 2.0: pixels (1024x768); image 2.1: --size 2K@16:9 (or --size 2K --ratio 16:9)"
	case provider.Gemini:
		return "pixels (1024x1024), ratio (16:9), or tier (2K); maps to aspect_ratio + image_size"
	case provider.ModelScope:
		return "pixels only (e.g. 1024x768)"
	case provider.Zeekai, provider.OpenLux, provider.Bailian, provider.Pollinations, provider.OpenAI:
		return "pixels (1024x1024) or ratio (16:9)"
	default:
		return "pixels (1024x1024) or ratio (16:9)"
	}
}

// fetchOpenRouterImageParams and the advisory GET live in
// client.OpenRouterImageParams, shared by the real request path and this hint.
