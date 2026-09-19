package image

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// savePromptFile saves the generation prompt to image_{taskID}.md.
func savePromptFile(taskID, prompt string) {
	if !options.Shared.SavePrompt {
		return
	}
	service.SavePrompt(options.Shared.OutputDir, taskID, prompt)
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

// GenerateAndSave generates images via the configured provider and saves them to disk.
// Handles config merge, timeout, API dispatch, and download. Returns paths to saved files.
// Shared by CLI (image command) and agent loop (chat) — single source of truth.
// Supports APIMart async and OpenAI-compatible sync providers.
func GenerateAndSave(c client.APIClient, req *types.GenerateRequest) ([]string, error) {
	// Always load the user's config — options.Shared.Cfg may be nil if PersistentPreRunE
	// hasn't run (e.g., direct call from agent loop without CLI entry).
	imgCfg := loadImageDefaults()

	if options.Shared.Verbose {
		if imgCfg != nil {
			fmt.Fprintf(os.Stderr, "\r\n[image] cfg: model=%s quality=%s size=%s res=%s\r\n",
				imgCfg.Model, imgCfg.Quality, imgCfg.Size, imgCfg.Resolution)
		} else {
			fmt.Fprintf(os.Stderr, "\r\n[image] WARNING: no image config loaded (check defaults.image in config.yaml)\r\n")
		}
		fmt.Fprintf(os.Stderr, "[image] req before: model=%s quality=%s size=%s res=%s\r\n",
			req.Model, req.Quality, req.Size, req.Resolution)
	}

	// Check if LLM is allowed to override config (default: false = config wins)
	allowOverride := false
	if cfg := options.ChatDefaults(); cfg != nil {
		allowOverride = cfg.AllowToolOverride
	}

	if imgCfg != nil {
		if !allowOverride {
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
		} else {
			// allow_tool_override=true — LLM takes priority, config only fills empty fields
			imgCfg.MergeIntoImage(req)
		}
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
		fmt.Fprintf(os.Stderr, "[image] req after:  model=%s quality=%s size=%s res=%s\r\n",
			req.Model, req.Quality, req.Size, req.Resolution)
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required: set via defaults.image.model in config.yaml")
	}

	// Set timeout
	options.ApplyTimeout(c, "image", client.ImageTimeout)

	// Resolve provider (named provider > global > builtin)
	p := options.Shared.ResolveProvider(options.ProviderNameImage)
	isAPIMart := options.IsAPIMartProvider(p)
	isOpenRouter := p.ProviderType == provider.OpenRouter
	isAgnes := p.ProviderType == provider.Agnes
	isOllama := p.Type == types.ProviderOllama || provider.IsLocalEndpoint(p.BaseURL)
	isModelScope := p.ProviderType == provider.ModelScope
	isGemini := p.ProviderType == provider.Gemini
	isZeekai := p.ProviderType == provider.Zeekai

	// Strip APIMart-only fields for non-APIMart providers.
	if !isAPIMart {
		req.Resolution = ""
	}

	if err := resolveRequestImages(c, req, isAPIMart); err != nil {
		return nil, err
	}

	// Strategy table: first match wins, last entry is the default.
	ictx := &imageDispatchCtx{
		isAPIMart:     isAPIMart,
		isOpenRouter:  isOpenRouter,
		isModelScope:  isModelScope,
		isAgnes:       isAgnes,
		isGemini:      isGemini,
		isZeekai:      isZeekai,
		genEdit:       false,
		isOllama:      isOllama,
		modelScopeKey: p.APIKey,
	}
	for _, s := range imageStrategies {
		if s.match(req, ictx) {
			return s.run(c, req, ictx)
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
		fmt.Fprintf(os.Stderr, "Warning: invalid --compress value %q: %v\n", genCompress, err)
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
			fmt.Fprintf(os.Stderr, "Warning: compress %s: %v\n", path, err)
			continue
		}
		if result.Skipped {
			fmt.Printf("Compress %s: skipped (%s)\n", path, result.Reason)
		} else {
			savedStr := formatBytes(result.After)
			originalStr := formatBytes(result.Before)
			pct := 100 - int(float64(result.After)/float64(result.Before)*100)
			params := formatParams(result.Format, result.Quality)
			fmt.Printf("Compress %s: %s → %s (%d%% saved)%s → %s\n", path, originalStr, savedStr, pct, params, result.DstPath)
		}
	}
}

// resolveRequestImages makes image inputs usable before dispatch, for both the
// typed path (APIMart upload) and a verbatim --json body (in-place rewrite).
func resolveRequestImages(c client.APIClient, req *types.GenerateRequest, isAPIMart bool) error {
	if isAPIMart {
		if len(req.ImageURLs) > 0 {
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}
		if req.MaskURL != "" {
			resolved, err := c.ResolveLocalImages([]string{req.MaskURL})
			if err != nil {
				return fmt.Errorf("failed to resolve mask-url: %w", err)
			}
			req.MaskURL = resolved[0]
		}
	}
	if len(req.RawJSON) == 0 {
		return nil
	}
	resolve := service.LocalFilesToDataURI
	if isAPIMart {
		resolve = c.ResolveLocalImages
	}
	patched, err := resolveRawImagePaths(req.RawJSON, resolve)
	if err != nil {
		return fmt.Errorf("failed to resolve image paths in JSON body: %w", err)
	}
	req.RawJSON = patched
	return nil
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
// so a verbatim --json body still accepts local files.
func resolveRawImagePaths(raw json.RawMessage, resolve func([]string) ([]string, error)) (json.RawMessage, error) {
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("failed to parse JSON body: %w", err)
	}
	changed := false
	for _, path := range rawImageKeyPaths {
		owner := body
		for _, part := range path[:len(path)-1] {
			nested, ok := owner[part].(map[string]interface{})
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
	return json.Marshal(body)
}

// resolveRawImageValue resolves one image field; ok is false when no local file
// is present and the value must be left untouched.
func resolveRawImageValue(v interface{}, resolve func([]string) ([]string, error)) (interface{}, bool, error) {
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
	case []interface{}:
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
		out := make([]interface{}, len(resolved))
		for i, s := range resolved {
			out[i] = s
		}
		return out, true, nil
	}
	return nil, false, nil
}
