package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// imageEditsImageField is the multipart field name OpenAI-family /images/edits
// endpoints expect for each source image.
const imageEditsImageField = "image"

// ImageGenerateEditsMultipart sends an OpenAI-family image edit request to
// POST /images/edits as multipart/form-data: one "image" part per reference
// image (plus an optional "mask") with the parameters as text fields. It is the
// edit protocol for endpoints (native OpenAI, Azure OpenAI, OfoxAI) whose
// /images/generations path defines no reference-image field.
func (c *Client) ImageGenerateEditsMultipart(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	contentType, body, err := buildImageEditsMultipart(req)
	if err != nil {
		return nil, err
	}
	var result types.OpenAIImageResponse
	if err := c.doBytes(http.MethodPost, ImageEditsPath, contentType, body, &result, provider.ImageEditsHeaders(c.baseURL)); err != nil {
		return nil, err
	}
	return &result, nil
}

// ImageEditsFormFields returns the ordered non-file multipart fields for a
// POST /images/edits body, omitting unset optional values. It is shared by the
// client request builder and the CLI --dry-run preview so both cannot drift.
//
// A verbatim --json body (used with --edit) is mapped to form fields: scalar
// keys become one field each and arrays of scalars become repeated fields. File
// keys are skipped because images are attached as file parts from -i.
func ImageEditsFormFields(req *types.GenerateRequest) []reqbuild.FormField {
	if len(req.RawJSON) > 0 {
		return imageEditsFieldsFromJSON(req.RawJSON)
	}
	fields := []reqbuild.FormField{
		{Name: "model", Value: req.Model},
		{Name: "prompt", Value: req.Prompt},
		{Name: "size", Value: req.Size},
		{Name: "quality", Value: req.Quality},
		{Name: "background", Value: req.Background},
		{Name: "output_format", Value: req.OutputFormat},
	}
	if req.N != nil {
		fields = append(fields, reqbuild.FormField{Name: "n", Value: strconv.Itoa(*req.N)})
	}
	out := make([]reqbuild.FormField, 0, len(fields))
	for _, f := range fields {
		if f.Value != "" {
			out = append(out, f)
		}
	}
	return out
}

// imageEditsFileKeys are JSON keys that carry image files (attached as parts,
// never as form fields).
var imageEditsFileKeys = map[string]bool{
	"image": true, "images": true, "image_url": true, "image_urls": true,
	"mask": true, "mask_url": true,
}

// imageEditsFieldsFromJSON maps a --json object to multipart form fields in a
// deterministic (sorted-key) order. Scalar values become one field; arrays of
// scalars become repeated fields; null and nested objects are skipped.
func imageEditsFieldsFromJSON(raw json.RawMessage) []reqbuild.FormField {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var body map[string]any
	if err := dec.Decode(&body); err != nil {
		return nil
	}
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var fields []reqbuild.FormField
	for _, k := range keys {
		if imageEditsFileKeys[k] {
			continue
		}
		switch v := body[k].(type) {
		case string:
			if v != "" {
				fields = append(fields, reqbuild.FormField{Name: k, Value: v})
			}
		case json.Number:
			fields = append(fields, reqbuild.FormField{Name: k, Value: v.String()})
		case bool:
			fields = append(fields, reqbuild.FormField{Name: k, Value: strconv.FormatBool(v)})
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					fields = append(fields, reqbuild.FormField{Name: k, Value: s})
				}
			}
		}
	}
	return fields
}

// buildImageEditsMultipart assembles the multipart/form-data body for
// POST /images/edits and returns its Content-Type plus bytes. It is pure (no
// network) unless an image is given as a remote URL or data URI, which must be
// fetched/decoded to be attached as a file part.
func buildImageEditsMultipart(req *types.GenerateRequest) (string, []byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for _, f := range ImageEditsFormFields(req) {
		if err := w.WriteField(f.Name, f.Value); err != nil {
			return "", nil, fmt.Errorf("failed to write %s field: %w", f.Name, err)
		}
	}

	for _, src := range req.ImageURLs {
		if err := writeImageEditsPart(w, imageEditsImageField, src); err != nil {
			return "", nil, err
		}
	}
	if strings.TrimSpace(req.MaskURL) != "" {
		if err := writeImageEditsPart(w, "mask", req.MaskURL); err != nil {
			return "", nil, err
		}
	}

	if err := w.Close(); err != nil {
		return "", nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}
	return w.FormDataContentType(), buf.Bytes(), nil
}

// writeImageEditsPart resolves one image source and appends it as a file part
// under field. Local files are read from disk; data URIs, raw base64, and
// remote URLs are fetched/decoded first.
func writeImageEditsPart(w *multipart.Writer, field, src string) error {
	data, filename, err := loadImageEditsSource(src)
	if err != nil {
		return err
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename))
	header.Set("Content-Type", http.DetectContentType(data))
	part, err := w.CreatePart(header)
	if err != nil {
		return fmt.Errorf("failed to create %s part: %w", field, err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("failed to write %s part: %w", field, err)
	}
	return nil
}

// loadImageEditsSource returns the raw bytes and a filename for one image
// source. Local paths are read directly; anything else (data URI, base64,
// http(s) URL) is resolved through service.FetchBytes.
func loadImageEditsSource(src string) ([]byte, string, error) {
	s := strings.TrimSpace(src)
	if isLocalFile(s) {
		data, err := os.ReadFile(s)
		if err != nil {
			return nil, "", fmt.Errorf("read image %s: %w", s, err)
		}
		return data, filepath.Base(s), nil
	}
	data, err := service.FetchBytes(s)
	if err != nil {
		return nil, "", fmt.Errorf("resolve image %s: %w", s, err)
	}
	return data, imageEditsFilenameFromMIME(http.DetectContentType(data)), nil
}

// imageEditsFilenameFromMIME maps a detected content type to a filename with a
// matching extension, so the uploaded part carries a plausible name.
func imageEditsFilenameFromMIME(contentType string) string {
	switch contentType {
	case "image/png":
		return "image.png"
	case "image/jpeg":
		return "image.jpg"
	case "image/webp":
		return "image.webp"
	case "image/gif":
		return "image.gif"
	default:
		return "image.png"
	}
}
