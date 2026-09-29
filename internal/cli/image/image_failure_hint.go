package image

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// sizeHintTimeout bounds the best-effort capability lookup performed while
// building a failure hint. The lookup is purely advisory, so it must never
// make a failed command hang.
const sizeHintTimeout = 5 * time.Second

// sizeErrorKeywords mark an API failure as (probably) size-related. Matching is
// case-insensitive. Kept narrow so unrelated 400s do not get noise appended.
var sizeErrorKeywords = []string{
	"size", "aspect", "ratio", "resolution", "dimension",
	"width", "height", "像素", "尺寸", "宽高比",
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
	params, found := fetchOpenRouterImageParams(base, req.Model)

	var b strings.Builder
	b.WriteString("\n\n  --size hint (OpenRouter):")
	switch {
	case !found:
		b.WriteString("\n    could not read this model's capabilities.")
		b.WriteString("\n    OpenRouter accepts an aspect ratio via --ratio (e.g. 16:9) or")
		b.WriteString("\n    a tier/pixels via --size (e.g. 2K, 1024x1024); --size 2K@16:9 sets both.")
	case len(params) == 0:
		fmt.Fprintf(&b, "\n    model %q declares no size parameter — remove --size / --ratio.", req.Model)
	default:
		if d, ok := params["aspect_ratio"]; ok && len(d.Values) > 0 {
			b.WriteString("\n    aspect ratio (--ratio): " + strings.Join(d.Values, " | "))
		}
		if d, ok := params["resolution"]; ok && len(d.Values) > 0 {
			b.WriteString("\n    resolution tier (--size): " + strings.Join(d.Values, " | "))
		}
		b.WriteString("\n    combine with --size <tier>@<ratio> (e.g. 2K@16:9).")
	}
	b.WriteString("\n    inspect: aigc-cli models --api-base " + base + " --type image")
	return b.String()
}

// staticSizeHint emits the documented accepted --size forms for providers that
// expose no machine-readable capability list.
func staticSizeHint(p *provider.EffectiveProvider) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n\n  --size hint (%s):", p.ProviderType.String())
	b.WriteString("\n    accepted: " + staticSizeForms(p.ProviderType))
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
		return "pixels (1024x1024); the CLI derives aspect_ratio from pixels"
	case provider.ModelScope:
		return "pixels only (e.g. 1024x768)"
	case provider.Zeekai, provider.OpenLux, provider.Bailian, provider.Pollinations, provider.OpenAI:
		return "pixels (1024x1024) or ratio (16:9)"
	default:
		return "pixels (1024x1024) or ratio (16:9)"
	}
}

// fetchOpenRouterImageParams returns the model's supported_parameters from
// OpenRouter's public image model discovery. found is false when the endpoint
// or model is unavailable, so the caller falls back to generic guidance.
func fetchOpenRouterImageParams(base, model string) (map[string]types.OpenRouterParamDescriptor, bool) {
	if base == "" || model == "" {
		return nil, false
	}
	body, err := hintGet(strings.TrimRight(base, "/") + "/images/models")
	if err != nil {
		return nil, false
	}
	var list types.OpenRouterMediaModelList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, false
	}
	for _, m := range list.Data {
		if m.ID == model {
			return m.SupportedParameters, true
		}
	}
	return nil, false
}

// hintGet performs a short, unauthenticated GET for advisory capability data.
// It uses http.DefaultClient so the configured proxy transport is honored.
func hintGet(rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sizeHintTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}
