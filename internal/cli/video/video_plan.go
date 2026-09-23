package video

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// videoSlotKind identifies which request image field an upload slot fills.
type videoSlotKind int

const (
	slotVidImageURL videoSlotKind = iota // req.ImageURLs[index]
	slotVidRoleURL                       // req.ImageWithRoles[index].URL
)

// videoUploadSlot maps one plan upload back to the request field it replaces.
type videoUploadSlot struct {
	path  string
	kind  videoSlotKind
	index int
}

// videoPlan is the per-provider request plan for video generation: the wire
// method/URL/body plus the local-image uploads that must run first. Preview
// (--dry-run/--verbose) and execution both derive from it, so they cannot drift.
type videoPlan struct {
	reqbuild.Plan
	slots []videoUploadSlot
}

// buildVideoPlan resolves the plan for req under p.
//
// Upload providers (APIMart, OpenLux) keep req untouched: their preview body is a
// shallow copy whose local values are placeholders, and applyUploads later
// rewrites req with the uploaded URLs. Encode providers (OpenRouter, Agnes)
// have no upload endpoint, so their local files are embedded as data: URIs in
// req in place — the same values the body and the runner then use.
func buildVideoPlan(req *types.VideoGenerateRequest, p *provider.EffectiveProvider) (*videoPlan, error) {
	base := client.NormalizeBaseURL(p.BaseURL)

	switch p.ProviderType {
	case provider.Pollinations:
		// GET <root>/video/{prompt}: no body and no uploads.
		return &videoPlan{Plan: reqbuild.Plan{
			Method: http.MethodGet,
			URL:    client.NewFromProvider(p).PollinationsVideoURL(req),
		}}, nil
	case provider.OpenRouter:
		if err := encodeVideoLocalImages(req); err != nil {
			return nil, err
		}
		// Known limitation (out of scope): every image_urls entry maps to
		// frame_type "first_frame"; OpenRouter's body carries no per-image role.
		return &videoPlan{Plan: reqbuild.Plan{
			Method: http.MethodPost,
			URL:    base + client.OpenRouterVideosPath,
			Body:   openRouterVideoBody(req),
		}}, nil
	case provider.Agnes:
		if err := encodeVideoLocalImages(req); err != nil {
			return nil, err
		}
		return &videoPlan{Plan: reqbuild.Plan{
			Method: http.MethodPost,
			URL:    base + client.AgnesVideoSubmitPath,
			Body:   client.AgnesVideoBody(req),
		}}, nil
	case provider.OpenLux:
		preview, uploads, slots := collectVideoUploads(req)
		return &videoPlan{
			Plan: reqbuild.Plan{
				Method:    http.MethodPost,
				URL:       base + client.OpenLuxVideoSubPath,
				Body:      client.OpenLuxVideoBody(preview),
				Uploads:   uploads,
				UploadURL: base + client.UploadPath,
			},
			slots: slots,
		}, nil
	default: // APIMart and any generic OpenAI-compatible relay
		preview, uploads, slots := collectVideoUploads(req)
		return &videoPlan{
			Plan: reqbuild.Plan{
				Method:    http.MethodPost,
				URL:       base + client.VideoSubmitPath,
				Body:      videoPreviewBody(preview),
				Uploads:   uploads,
				UploadURL: base + client.UploadPath,
			},
			slots: slots,
		}, nil
	}
}

// videoPreviewRequest is a shallow copy of VideoGenerateRequest used only for
// the APIMart/generic preview. Because it drops VideoGenerateRequest's
// MarshalJSON method, reqbuild can marshal it with HTML escaping off, keeping
// placeholder tokens copy-pasteable.
type videoPreviewRequest types.VideoGenerateRequest

// videoPreviewBody selects the APIMart/generic preview body. A verbatim --json
// body wins over the typed fields, exactly as VideoGenerateRequest.MarshalJSON
// does when the real request is sent.
func videoPreviewBody(preview *types.VideoGenerateRequest) any {
	if len(preview.RawJSON) > 0 {
		return json.RawMessage(preview.RawJSON)
	}
	shadow := videoPreviewRequest(*preview)
	return &shadow
}

// encodeVideoLocalImages rewrites local-file entries to data: URIs in place.
// Values already carrying a data: URI, or an http(s) URL, pass through.
func encodeVideoLocalImages(req *types.VideoGenerateRequest) error {
	encoded, err := service.LocalFilesToDataURI(req.ImageURLs)
	if err != nil {
		return fmt.Errorf("failed to encode image-urls: %w", err)
	}
	req.ImageURLs = encoded
	for i := range req.ImageWithRoles {
		resolved, err := service.LocalFilesToDataURI([]string{req.ImageWithRoles[i].URL})
		if err != nil {
			return fmt.Errorf("failed to encode image-with-role: %w", err)
		}
		req.ImageWithRoles[i].URL = resolved[0]
	}
	return nil
}

// collectVideoUploads records one upload per local image and returns a shallow
// copy of req whose local values are replaced by plan placeholders.
func collectVideoUploads(req *types.VideoGenerateRequest) (*types.VideoGenerateRequest, []reqbuild.Upload, []videoUploadSlot) {
	preview := *req
	var uploads []reqbuild.Upload
	var slots []videoUploadSlot

	if len(req.ImageURLs) > 0 {
		urls := append([]string(nil), req.ImageURLs...)
		for i, u := range urls {
			if !service.IsFile(u) {
				continue
			}
			uploads, slots = appendVideoUpload(uploads, slots, u, slotVidImageURL, i)
			urls[i] = uploads[len(uploads)-1].Placeholder
		}
		preview.ImageURLs = urls
	}
	if len(req.ImageWithRoles) > 0 {
		roles := append([]types.ImageWithRole(nil), req.ImageWithRoles...)
		for i, r := range roles {
			if !service.IsFile(r.URL) {
				continue
			}
			uploads, slots = appendVideoUpload(uploads, slots, r.URL, slotVidRoleURL, i)
			roles[i].URL = uploads[len(uploads)-1].Placeholder
		}
		preview.ImageWithRoles = roles
	}
	return &preview, uploads, slots
}

// appendVideoUpload records one upload and returns the extended parallel slices.
func appendVideoUpload(uploads []reqbuild.Upload, slots []videoUploadSlot, path string, kind videoSlotKind, index int) ([]reqbuild.Upload, []videoUploadSlot) {
	placeholder := reqbuild.Placeholder(len(uploads))
	label := fmt.Sprintf("image_urls[%d]", index)
	if kind == slotVidRoleURL {
		label = fmt.Sprintf("image_with_roles[%d].url", index)
	}
	uploads = append(uploads, reqbuild.Upload{Path: path, Placeholder: placeholder, Label: label})
	slots = append(slots, videoUploadSlot{path: path, kind: kind, index: index})
	return uploads, slots
}

// applyUploads runs the plan's uploads and writes the resolved URLs back into
// req. It is a no-op when the plan has no uploads.
func (pl *videoPlan) applyUploads(c client.APIClient, req *types.VideoGenerateRequest) error {
	if len(pl.Uploads) == 0 {
		return nil
	}
	resolved, err := pl.RunUploads(c)
	if err != nil {
		return err
	}
	if len(resolved) != len(pl.slots) {
		return fmt.Errorf("upload resolution returned %d URLs for %d slots", len(resolved), len(pl.slots))
	}
	for i, slot := range pl.slots {
		switch slot.kind {
		case slotVidImageURL:
			req.ImageURLs[slot.index] = resolved[i]
		case slotVidRoleURL:
			req.ImageWithRoles[slot.index].URL = resolved[i]
		default:
			return fmt.Errorf("unknown upload slot kind %d for %s", slot.kind, slot.path)
		}
	}
	return nil
}
