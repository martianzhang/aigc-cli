package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/clientfactory"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// savePromptFile saves the generation prompt to image_{taskID}.md.
func savePromptFile(taskID, prompt string) {
	if !options.Shared.SavePrompt {
		return
	}
	if err := service.SavePrompt(options.Shared.OutputDir, "image", taskID, prompt); err != nil {
		ui.Eprintf("Warning: %v\n", err)
	}
}

// imageStamp returns the provider's created timestamp when present, otherwise
// a millisecond clock value. Some OpenAI-compatible relays omit `created`;
// using the zero value would collapse every file to `<prefix>_0` and overwrite
// the previous generation.
func imageStamp(created int64) int64 {
	if created > 0 {
		return created
	}
	return time.Now().UnixMilli()
}

// promptTaskID builds the task ID shared by saved prompt and image files.
// created is the provider-returned timestamp; 0 falls back to a unique clock
// value so repeated generations never reuse the same filename.
func promptTaskID(prefix string, created int64) string {
	return fmt.Sprintf("%s_%d", prefix, imageStamp(created))
}

// loadImageDefaults returns the user's image config defaults.
// Tries options.Shared.Cfg first (fast), falls back to reading from file.
func loadImageDefaults() *types.ImageDefaults {
	if cfg := options.ImageDefaults(); cfg != nil {
		return cfg
	}
	// Fallback: load from file directly
	if cfg, err := config.Load(options.Shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
		return cfg.Defaults.Image
	}
	return nil
}

// GenerateAndSave generates images via p and saves them to disk.
// Handles config merge, timeout, API dispatch, and download. Returns paths to saved files.
//
// p must be the provider that also built c. The MCP handler resolves an
// optional per-call `provider` argument before calling this function; passing
// it through is what keeps the strategy (sync vs ModelScope/APIMart async),
// the endpoint and the API key on the same backend. A nil p falls back to the
// configured image provider for older callers.
func GenerateAndSave(c client.APIClient, p *provider.EffectiveProvider, req *types.GenerateRequest) ([]string, error) {
	// Always load the user's config — options.Shared.Cfg may be nil if PersistentPreRunE
	// hasn't run (e.g., direct call from agent loop without CLI entry).
	imgCfg := loadImageDefaults()

	if options.Shared.Verbose {
		if imgCfg != nil {
			ui.Eprintf("\r\n[image] cfg: model=%s quality=%s size=%s res=%s\r\n",
				imgCfg.Model, imgCfg.Quality, imgCfg.Size, imgCfg.Resolution)
		} else {
			ui.Eprintf("\r\n[image] WARNING: no image config loaded (check defaults.image in config.yaml)\r\n")
		}
		ui.Eprintf("[image] req before: model=%s quality=%s size=%s res=%s\r\n",
			req.Model, req.Quality, req.Size, req.Resolution)
	}

	// The configured default decides whether the caller explicitly switched
	// providers for this call. If they did, defaults.image.* belongs to a
	// different backend and must not be forced onto the request (in particular
	// its model would otherwise be sent to the wrong vendor).
	configured := options.Shared.ResolveProvider(options.ProviderNameImage)
	if p == nil {
		p = configured
	}
	explicitProvider := p.Name != "" && p.Name != configured.Name

	// Check if LLM is allowed to override config (default: false = config wins)
	allowOverride := false
	if cfg := options.ChatDefaults(); cfg != nil {
		allowOverride = cfg.AllowToolOverride
	}

	switch {
	case explicitProvider:
		// Only the selected provider's own model applies.
		if req.Model == "" {
			req.Model = p.Model
		}
	case imgCfg != nil && !allowOverride:
		// Config is the ceiling — force values regardless of LLM
		if imgCfg.Model != "" {
			req.Model = imgCfg.Model
		}
		if imgCfg.Quality != "" {
			req.Quality = imgCfg.Quality
		}
		if imgCfg.Size != "" {
			req.Size = imgCfg.Size
		}
		if imgCfg.Resolution != "" {
			req.Resolution = imgCfg.Resolution
		}
	case imgCfg != nil:
		// allow_tool_override=true — LLM takes priority, config only fills empty fields
		imgCfg.MergeIntoImage(req)
	}
	// Code defaults for fields the user didn't configure
	if req.Size == "" {
		req.Size = "1:1"
	}
	if req.Quality == "" {
		req.Quality = "low"
	}
	if req.Resolution == "" {
		req.Resolution = "1k"
	}
	if options.Shared.Verbose {
		ui.Eprintf("[image] req after:  model=%s quality=%s size=%s res=%s\r\n",
			req.Model, req.Quality, req.Size, req.Resolution)
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required: set model in the request, providers.<name>.model, or defaults.image.model in config.yaml")
	}

	// Set timeout
	clientfactory.ApplyTimeout(c, "image", client.ImageTimeout)

	// Strip APIMart-only fields for non-APIMart providers.
	if !options.IsAPIMartProvider(p) {
		req.Resolution = ""
	}

	plan, err := buildImagePlan(req, p)
	if err != nil {
		return nil, err
	}
	if err := plan.applyUploads(c, req); err != nil {
		return nil, err
	}

	// Strategy table: first match wins, last entry is the default.
	ictx := newImageDispatchCtx(p, req, false)
	for _, s := range imageStrategies {
		if s.match(req, ictx) {
			saved, err := s.run(c, req, ictx)
			if err != nil {
				if hint := imageFailureHint(p, req, err); hint != "" {
					return nil, fmt.Errorf("%w%s", err, hint)
				}
			}
			return saved, err
		}
	}
	return nil, fmt.Errorf("no image strategy matched")
}

// postProcessImages applies --compress post-processing to already-saved images.
// Called after image generation/download from all 4 save points.
func postProcessImages(saved []string) {
	if genCompress == "" || len(saved) == 0 {
		return
	}
	targetSize, quality, err := service.ParseCompressOption(genCompress)
	if err != nil {
		ui.Eprintf("Warning: invalid --compress value %q: %v\n", genCompress, err)
		return
	}
	opts := &service.CompressOptions{
		TargetSize: targetSize,
		Quality:    quality,
		Format:     genOutputFormat,
	}
	for _, path := range saved {
		result, err := service.CompressImage(path, opts)
		if err != nil {
			ui.Eprintf("Warning: compress %s: %v\n", path, err)
			continue
		}
		if result.Skipped {
			ui.Printf("Compress %s: skipped (%s)\n", path, result.Reason)
		} else {
			savedStr := formatBytes(result.After)
			originalStr := formatBytes(result.Before)
			pct := 100 - int(float64(result.After)/float64(result.Before)*100)
			params := formatParams(result.Format, result.Quality)
			ui.Printf("Compress %s: %s → %s (%d%% saved)%s → %s\n", path, originalStr, savedStr, pct, params, result.DstPath)
		}
	}
}

// rawImageKeyPaths are the nested image fields a verbatim --json body may use,
// across providers (top-level for OpenAI-style, extra_body.image for Agnes).
var rawImageKeyPaths = [][]string{
	{"image_url"},
	{"image_urls"},
	{"mask_url"},
	{"extra_body", "image"},
}

// resolveRawImagePaths rewrites local file paths under a raw body's image keys
// so a verbatim --json body still accepts local files. Numbers and special
// characters survive the round trip: decode uses json.Number and encode leaves
// HTML escaping off, matching MergeJSONOverlay's fidelity guarantees.
func resolveRawImagePaths(raw json.RawMessage, resolve func([]string) ([]string, error)) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return nil, fmt.Errorf("failed to parse JSON body: %w", err)
	}
	changed := false
	for _, path := range rawImageKeyPaths {
		owner := body
		for _, part := range path[:len(path)-1] {
			nested, ok := owner[part].(map[string]any)
			if !ok {
				owner = nil
				break
			}
			owner = nested
		}
		if owner == nil {
			continue
		}
		key := path[len(path)-1]
		updated, ok, err := resolveRawImageValue(owner[key], resolve)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		owner[key] = updated
		changed = true
	}
	if !changed {
		return raw, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		return nil, fmt.Errorf("failed to encode JSON body: %w", err)
	}
	return json.RawMessage(bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})), nil
}

// resolveRawImageValue resolves one image field; ok is false when no local file
// is present and the value must be left untouched.
func resolveRawImageValue(v any, resolve func([]string) ([]string, error)) (any, bool, error) {
	switch t := v.(type) {
	case string:
		if !service.IsFile(t) {
			return nil, false, nil
		}
		resolved, err := resolve([]string{t})
		if err != nil {
			return nil, false, err
		}
		return resolved[0], true, nil
	case []any:
		paths := make([]string, len(t))
		hasFile := false
		for i, item := range t {
			s, _ := item.(string)
			paths[i] = s
			if service.IsFile(s) {
				hasFile = true
			}
		}
		if !hasFile {
			return nil, false, nil
		}
		resolved, err := resolve(paths)
		if err != nil {
			return nil, false, err
		}
		out := make([]any, len(resolved))
		for i, s := range resolved {
			out[i] = s
		}
		return out, true, nil
	}
	return nil, false, nil
}
