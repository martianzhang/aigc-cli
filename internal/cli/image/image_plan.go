package image

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// imageSlotKind identifies where a resolved upload URL must be written back.
type imageSlotKind int

const (
	slotImageURL imageSlotKind = iota // into req.ImageURLs[index]
	slotMaskURL                       // into req.MaskURL
	slotRawJSON                       // into the verbatim --json body
)

// imageUploadSlot records one pending upload and its write-back target.
type imageUploadSlot struct {
	path  string
	kind  imageSlotKind
	index int // index into req.ImageURLs for slotImageURL
}

// imagePlan is a provider request plan plus the upload write-back map.
type imagePlan struct {
	reqbuild.Plan
	slots []imageUploadSlot
}

// buildImagePlan resolves the endpoint, preview body, and pre-uploads for req.
// It reads local files to embed data URIs but performs no network I/O, so it is
// safe for --dry-run/--verbose and is the single source of truth for execution.
func buildImagePlan(req *types.GenerateRequest, p *provider.EffectiveProvider) (*imagePlan, error) {
	if p == nil {
		return nil, fmt.Errorf("provider is required")
	}

	// Expand a compound "<tier>@<ratio>" size (e.g. "2K@16:9") before any
	// dispatch so every entry path (CLI, MCP/chat agent, --dry-run) sees the split.
	if err := normalizeSizeTier(req); err != nil {
		return nil, err
	}

	base := client.NormalizeBaseURL(p.BaseURL)
	// APIMart precedes the local/Ollama check to mirror the strategy table,
	// where an APIMart-typed provider outranks a local endpoint.
	if options.IsAPIMartProvider(p) {
		return buildAPIMartPlan(req, base)
	}
	if p.Type == types.ProviderOllama || provider.IsLocalEndpoint(p.BaseURL) {
		body, err := service.OllamaGenerateBody(req)
		if err != nil {
			return nil, err
		}
		return newImagePlan(service.OllamaGenerateURL(p.BaseURL), body), nil
	}

	switch p.ProviderType {
	case provider.OpenRouter:
		if err := encodeLocalImages(req); err != nil {
			return nil, err
		}
		body, err := client.OpenRouterImageBody(req)
		if err != nil {
			return nil, err
		}
		return newImagePlan(base+client.OpenRouterImagesPath, body), nil
	case provider.Gemini:
		if err := encodeLocalImages(req); err != nil {
			return nil, err
		}
		body, err := client.GeminiImageBody(req)
		if err != nil {
			return nil, err
		}
		return newImagePlan(base+client.GeminiInteractionsPath, body), nil
	case provider.ModelScope:
		if err := encodeLocalImages(req); err != nil {
			return nil, err
		}
		body, err := buildModelScopeImageBody(req)
		if err != nil {
			return nil, err
		}
		injected, err := types.MaybeInject(body, req.InjectJSON)
		if err != nil {
			return nil, err
		}
		return newImagePlan(base+client.ImageSubmitPath, injected), nil
	case provider.Zeekai:
		if usesImageEditsJSON(p, req) {
			if err := encodeLocalImages(req); err != nil {
				return nil, err
			}
			body, err := client.ImageEditsBody(req)
			if err != nil {
				return nil, err
			}
			pl := newImagePlan(base+client.ImageEditsPath, body)
			pl.Note = imageEditsDataURINote
			return pl, nil
		}
	case provider.Agnes:
		if err := prepareAgnesImageRequest(req); err != nil {
			return nil, err
		}
		return newImagePlan(base+client.ImageSubmitPath, req), nil
	}

	// Default: OpenAI-compatible synchronous generation. OpenAI has no image
	// field, but compatible relays may accept image_urls, and a self-contained
	// data URI is strictly better than a local path that never worked.
	//
	// OpenAI-family endpoints (native OpenAI, Azure, OfoxAI) are the exception:
	// their /images/generations path has no reference-image field at all, so an
	// input image must go to POST /images/edits as multipart/form-data. A
	// verbatim --json body keeps the JSON sync path untouched.
	if useMultipartImageEdits(p, req) {
		return buildMultipartImageEditsPlan(req, base), nil
	}
	if err := encodeLocalImages(req); err != nil {
		return nil, err
	}
	return newImagePlan(base+client.ImageSubmitPath, req), nil
}

// newImagePlan builds a POST plan with no uploads.
func newImagePlan(rawURL string, body any) *imagePlan {
	return &imagePlan{Plan: reqbuild.Plan{Method: http.MethodPost, URL: rawURL, Body: body}}
}

// imageMultipartEditsNote is the plain-text note reqbuild renders as
// "# note: <text>" for the OpenAI-family multipart /images/edits plan.
const imageMultipartEditsNote = "each reference image is uploaded as a multipart/form-data file part (image=@<file>)"

// buildMultipartImageEditsPlan builds the multipart/form-data plan for
// POST /images/edits used by OpenAI-family endpoints. Local files become file
// parts; a remote URL or data URI is resolved at send time. The non-file fields
// come from client.ImageEditsFormFields, the same builder the real request uses.
func buildMultipartImageEditsPlan(req *types.GenerateRequest, base string) *imagePlan {
	pl := &imagePlan{Plan: reqbuild.Plan{
		Method:     http.MethodPost,
		URL:        base + client.ImageEditsPath,
		Multipart:  true,
		FormFields: client.ImageEditsFormFields(req),
		Note:       imageMultipartEditsNote,
	}}
	headers := provider.ImageEditsHeaders(base)
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pl.Headers = append(pl.Headers, reqbuild.Header{Name: name, Value: headers[name]})
	}
	for _, src := range req.ImageURLs {
		pl.FormFiles = append(pl.FormFiles, reqbuild.FormFile{Field: "image", Path: src})
	}
	if strings.TrimSpace(req.MaskURL) != "" {
		pl.FormFiles = append(pl.FormFiles, reqbuild.FormFile{Field: "mask", Path: req.MaskURL})
	}
	return pl
}

// encodeLocalImages converts local image paths to data URIs in place, across
// typed fields and a verbatim --json body. Public URLs and data URIs pass
// through unchanged, so the conversion is idempotent.
func encodeLocalImages(req *types.GenerateRequest) error {
	if len(req.ImageURLs) > 0 {
		resolved, err := service.LocalFilesToDataURI(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to encode image-urls: %w", err)
		}
		req.ImageURLs = resolved
	}
	if req.MaskURL != "" {
		resolved, err := service.LocalFilesToDataURI([]string{req.MaskURL})
		if err != nil {
			return fmt.Errorf("failed to encode mask-url: %w", err)
		}
		req.MaskURL = resolved[0]
	}
	if len(req.RawJSON) > 0 {
		patched, err := resolveRawImagePaths(req.RawJSON, service.LocalFilesToDataURI)
		if err != nil {
			return fmt.Errorf("failed to encode image paths in JSON body: %w", err)
		}
		req.RawJSON = patched
	}
	return nil
}
