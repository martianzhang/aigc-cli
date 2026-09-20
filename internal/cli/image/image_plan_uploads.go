package image

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// imageUploadField is the multipart field name APIMart's upload endpoint expects.
const imageUploadField = "file"

// imagePreview is a shallow copy of GenerateRequest used only for the APIMart
// preview. Because it drops GenerateRequest's MarshalJSON method, reqbuild can
// marshal it with HTML escaping off, keeping placeholder tokens copy-pasteable.
type imagePreview types.GenerateRequest

// buildAPIMartPlan records one upload per local file and builds the preview
// body from a copy whose local values are replaced with placeholder tokens.
// The request itself is left untouched until applyUploads runs.
func buildAPIMartPlan(req *types.GenerateRequest, base string) (*imagePlan, error) {
	pl := &imagePlan{Plan: reqbuild.Plan{
		Method:    http.MethodPost,
		URL:       base + client.ImageSubmitPath,
		UploadURL: base + client.UploadPath,
	}}

	preview := imagePreview(*req)
	preview.ImageURLs = append([]string(nil), req.ImageURLs...)

	// A verbatim --json body wins over the typed fields, exactly as
	// GenerateRequest.MarshalJSON does when the real request is sent.
	if len(preview.RawJSON) > 0 {
		raw, err := pl.recordRawUploads(preview.RawJSON)
		if err != nil {
			return nil, err
		}
		pl.Body = json.RawMessage(raw)
		return pl, nil
	}

	for i, value := range preview.ImageURLs {
		if !service.IsFile(value) {
			continue
		}
		placeholder := pl.recordUpload(value, slotImageURL, i, fmt.Sprintf("image_urls[%d]", i))
		preview.ImageURLs[i] = placeholder
	}
	if service.IsFile(preview.MaskURL) {
		preview.MaskURL = pl.recordUpload(preview.MaskURL, slotMaskURL, 0, "mask_url")
	}

	pl.Body = &preview
	return pl, nil
}

// recordUpload appends an upload plus its write-back slot and returns the
// placeholder token to embed in the preview body.
func (pl *imagePlan) recordUpload(path string, kind imageSlotKind, index int, label string) string {
	placeholder := reqbuild.Placeholder(len(pl.Uploads))
	pl.Uploads = append(pl.Uploads, reqbuild.Upload{
		Path:        path,
		Field:       imageUploadField,
		Placeholder: placeholder,
		Label:       label,
	})
	pl.slots = append(pl.slots, imageUploadSlot{path: path, kind: kind, index: index})
	return placeholder
}

// recordRawUploads replaces local file paths under a verbatim --json body's
// image keys with placeholders, recording an upload for each.
func (pl *imagePlan) recordRawUploads(raw []byte) ([]byte, error) {
	resolve := func(paths []string) ([]string, error) {
		out := make([]string, len(paths))
		for i, path := range paths {
			if !service.IsFile(path) {
				out[i] = path
				continue
			}
			out[i] = pl.recordUpload(path, slotRawJSON, 0, "json image")
		}
		return out, nil
	}
	return resolveRawImagePaths(raw, resolve)
}

// applyUploads uploads every local file in the plan and writes the resolved
// URLs back into req (typed fields and a verbatim --json body), then points
// Body at req so --verbose prints the same payload execution sends.
func (pl *imagePlan) applyUploads(c client.APIClient, req *types.GenerateRequest) error {
	if len(pl.slots) == 0 {
		return nil
	}
	resolved, err := pl.RunUploads(c)
	if err != nil {
		return err
	}
	urlByPath := make(map[string]string, len(pl.slots))
	for i, slot := range pl.slots {
		if i < len(resolved) {
			urlByPath[slot.path] = resolved[i]
		}
	}

	hasRaw := false
	for _, slot := range pl.slots {
		switch slot.kind {
		case slotImageURL:
			if slot.index < len(req.ImageURLs) {
				req.ImageURLs[slot.index] = urlByPath[slot.path]
			}
		case slotMaskURL:
			req.MaskURL = urlByPath[slot.path]
		case slotRawJSON:
			hasRaw = true
		}
	}
	if hasRaw {
		patched, err := resolveRawImagePaths(req.RawJSON, func(paths []string) ([]string, error) {
			out := make([]string, len(paths))
			for i, path := range paths {
				if url, ok := urlByPath[path]; ok {
					out[i] = url
				} else {
					out[i] = path
				}
			}
			return out, nil
		})
		if err != nil {
			return fmt.Errorf("failed to write uploaded URLs into JSON body: %w", err)
		}
		req.RawJSON = patched
	}

	pl.Body = req
	return nil
}
