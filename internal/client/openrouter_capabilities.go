package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/martianzhang/aigc-cli/internal/httpx"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	// openRouterParamsTimeout bounds the advisory capability lookup so a slow
	// discovery endpoint can never stall a generation request.
	openRouterParamsTimeout = 5 * time.Second
	// openRouterParamsTTL caches a successful lookup; capabilities change rarely.
	openRouterParamsTTL = 24 * time.Hour
	// openRouterParamsMissTTL re-checks a model that was not listed, so a newly
	// published model becomes visible without restarting the process.
	openRouterParamsMissTTL = 5 * time.Minute
	// openRouterParamsMaxBody caps the discovery response (it lists every model).
	openRouterParamsMaxBody int64 = 4 << 20
)

// cachedOpenRouterParams is one entry of the capability cache.
type cachedOpenRouterParams struct {
	params map[string]types.OpenRouterParamDescriptor
	found  bool
	at     time.Time
}

// openRouterParamsCache keys "<normalized base>\x00<model id>".
var openRouterParamsCache sync.Map

// OpenRouterImageParams returns the request fields an OpenRouter image model
// declares in GET /images/models. found is false when the endpoint or the model
// is unavailable; callers then send the request unchanged and let the API
// report any unsupported field.
func OpenRouterImageParams(baseURL, modelID string) (map[string]types.OpenRouterParamDescriptor, bool) {
	if v, ok := cachedOpenRouterParamsEntry(baseURL, modelID); ok {
		return v.params, v.found
	}
	base := NormalizeBaseURL(baseURL)
	params, found := fetchOpenRouterImageParams(base, modelID)
	openRouterParamsCache.Store(cacheKey(base, modelID), cachedOpenRouterParams{
		params: params,
		found:  found,
		at:     time.Now(),
	})
	return params, found
}

// CachedOpenRouterImageParams returns an already-fetched capability entry
// without network I/O, so --dry-run stays offline.
func CachedOpenRouterImageParams(baseURL, modelID string) (map[string]types.OpenRouterParamDescriptor, bool) {
	v, ok := cachedOpenRouterParamsEntry(baseURL, modelID)
	if !ok {
		return nil, false
	}
	return v.params, v.found
}

// cachedOpenRouterParamsEntry returns a non-expired cache entry.
func cachedOpenRouterParamsEntry(baseURL, modelID string) (cachedOpenRouterParams, bool) {
	if baseURL == "" || modelID == "" {
		return cachedOpenRouterParams{}, false
	}
	key := cacheKey(NormalizeBaseURL(baseURL), modelID)
	raw, ok := openRouterParamsCache.Load(key)
	if !ok {
		return cachedOpenRouterParams{}, false
	}
	entry := raw.(cachedOpenRouterParams)
	ttl := openRouterParamsTTL
	if !entry.found {
		ttl = openRouterParamsMissTTL
	}
	if time.Since(entry.at) >= ttl {
		return cachedOpenRouterParams{}, false
	}
	return entry, true
}

func cacheKey(base, modelID string) string { return base + "\x00" + modelID }

// fetchOpenRouterImageParams performs the discovery GET. It uses
// http.DefaultClient so the configured proxy transport is honored.
func fetchOpenRouterImageParams(base, modelID string) (map[string]types.OpenRouterParamDescriptor, bool) {
	if base == "" || modelID == "" {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), openRouterParamsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/images/models", nil)
	if err != nil {
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := httpx.ReadAllLimited(resp.Body, openRouterParamsMaxBody)
	if err != nil {
		return nil, false
	}
	var list types.OpenRouterMediaModelList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, false
	}
	for _, model := range list.Data {
		if model.ID == modelID {
			return model.SupportedParameters, true
		}
	}
	return nil, false
}

// FilterOpenRouterImageRequest removes fields the model does not declare in
// supported_parameters. OpenRouter rejects an undeclared field (for example
// "Tencent HY-World panorama does not support: size, output_format, quality"),
// so a model that declares only `n` must be called with model+prompt alone.
//
// The request is not mutated. A verbatim --json body is returned untouched
// because it is the caller's explicit wire format.
func FilterOpenRouterImageRequest(req *types.GenerateRequest, params map[string]types.OpenRouterParamDescriptor) (*types.GenerateRequest, error) {
	if req == nil || len(req.RawJSON) > 0 || len(params) == 0 {
		return req, nil
	}
	has := func(name string) bool {
		_, ok := params[name]
		return ok
	}
	out := *req

	if len(out.ImageURLs) > 0 && !has("input_references") {
		return nil, fmt.Errorf("model %q does not accept reference images (input_references is not supported)", out.Model)
	}

	switch {
	case has("size"):
		if !has("aspect_ratio") {
			out.Ratio = ""
		}
	case has("aspect_ratio") || has("resolution"):
		// Without a `size` field, map a ratio-shaped size onto aspect_ratio and
		// a tier-shaped size onto resolution, so --size 16:9 / --size 2K keeps
		// working on models that expose only those two knobs.
		if !has("aspect_ratio") {
			out.Ratio = ""
		}
		if out.Ratio == "" && has("aspect_ratio") && isAspectRatioValue(out.Size) {
			out.Ratio = out.Size
		}
		if has("resolution") && isTierValue(out.Size) {
			out.Resolution = out.Size
		}
		out.Size = ""
	default:
		out.Size = ""
		out.Ratio = ""
	}

	if !has("quality") {
		out.Quality = ""
	}
	if !has("output_format") {
		out.OutputFormat = ""
	}
	if !has("background") {
		out.Background = ""
	}
	if !has("output_compression") {
		out.OutputCompression = nil
	}
	if !has("resolution") {
		out.Resolution = ""
	}
	if !has("complexity") {
		out.Complexity = ""
	}
	if !has("n") {
		if out.N != nil && *out.N > 1 {
			return nil, fmt.Errorf("model %q does not support n > 1", out.Model)
		}
		out.N = nil
	}
	return &out, nil
}

// isAspectRatioValue reports whether v looks like "16:9" rather than a tier
// ("2K") or an explicit pixel size ("1024x1024").
func isAspectRatioValue(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && !isPixelSize(v) && strings.Contains(v, ":")
}

// isTierValue reports whether v is one of OpenRouter's normalized resolution
// tiers, so a --size tier can be sent as `resolution` on models that do not
// declare `size`.
func isTierValue(v string) bool {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "512", "768", "1K", "1.5K", "2K", "4K":
		return true
	}
	return false
}
