// Package reqbuild holds the provider-agnostic request-plan primitives shared
// by --dry-run/--verbose previews and real execution, so a rendered curl and
// the call it describes come from one source of truth.
package reqbuild

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/service"
)

// defaultUploadField is the multipart field name used when Upload.Field is empty.
const defaultUploadField = "file"

// Upload describes one local file that must be uploaded before the request.
type Upload struct {
	Path        string // local file path to upload
	Field       string // multipart field name; defaults to "file"
	Placeholder string // token embedded in the preview body
	Label       string // preview comment, e.g. "image_urls[0]"
}

// Plan is a provider-agnostic request: what to send and which local files to
// upload first. It is render-only until RunUploads is called.
type Plan struct {
	Method    string   // http.MethodGet or http.MethodPost
	URL       string   // absolute request URL
	Body      any      // nil for GET
	Uploads   []Upload // ordered
	UploadURL string   // absolute upload endpoint (render-only)
	Note      string   // optional trailing preview comment
}

// Uploader resolves local image paths to public URLs. *client.Client satisfies it.
type Uploader interface {
	ResolveLocalImages(urls []string) ([]string, error)
}

// Placeholder returns the token a caller embeds in a preview body for the
// i-th upload, e.g. "<UPLOAD_URL_0>".
func Placeholder(i int) string {
	return "<UPLOAD_URL_" + strconv.Itoa(i) + ">"
}

// RenderCurls renders the equivalent curl commands for the plan: one multipart
// upload per Upload, then the request. It is pure and performs no I/O.
func (p *Plan) RenderCurls(apiKey string) string {
	blocks := make([]string, 0, len(p.Uploads)+1)
	for _, u := range p.Uploads {
		blocks = append(blocks, renderUploadCurl(u, p.UploadURL, apiKey))
	}
	blocks = append(blocks, p.renderRequestCurl(apiKey))
	return strings.Join(blocks, "\n")
}

// renderUploadCurl renders one multipart upload. It deliberately omits a
// Content-Type header: curl derives the multipart boundary itself.
func renderUploadCurl(u Upload, uploadURL, apiKey string) string {
	field := u.Field
	if field == "" {
		field = defaultUploadField
	}
	lines := []string{fmt.Sprintf("curl -X POST %s", uploadURL)}
	lines = appendAuthLine(lines, apiKey)
	lines = append(lines, fmt.Sprintf("  -F \"%s=@%s\"", field, u.Path))
	return joinCurlLines(lines)
}

// renderRequestCurl renders the generation request itself.
func (p *Plan) renderRequestCurl(apiKey string) string {
	lines := []string{fmt.Sprintf("curl -X %s %s", p.Method, p.URL)}
	lines = appendAuthLine(lines, apiKey)
	if p.Method != http.MethodGet {
		lines = append(lines, "  -H \"Content-Type: application/json\"")
		data, _ := marshalBody(p.Body)
		lines = append(lines, fmt.Sprintf("  -d '%s'", string(data)))
	}
	cmd := joinCurlLines(lines)
	if p.Note != "" {
		cmd += "\n# note: " + p.Note
	}
	return cmd
}

// appendAuthLine appends the masked Authorization header when a key is present.
func appendAuthLine(lines []string, apiKey string) []string {
	if apiKey == "" {
		return lines
	}
	return append(lines, fmt.Sprintf("  -H \"Authorization: Bearer %s\"", service.MaskKey(apiKey)))
}

// joinCurlLines joins command lines with a trailing backslash continuation.
// The final line has no continuation.
func joinCurlLines(lines []string) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteString(" \\\n")
		}
		b.WriteString(line)
	}
	return b.String()
}

// marshalBody marshals a preview body, emitting pre-marshalled bodies verbatim
// so a --json request cannot be reshaped by the preview. HTML escaping is off
// so upload placeholder tokens stay copy-pasteable in the rendered curl.
func marshalBody(body any) ([]byte, error) {
	switch b := body.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		return b, nil
	case []byte:
		return b, nil
	default:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(body); err != nil {
			return nil, err
		}
		return bytes.TrimRight(buf.Bytes(), "\n"), nil
	}
}

// RunUploads is the only network-touching method: it uploads every local file
// in Uploads order and returns the resolved URLs.
func (p *Plan) RunUploads(u Uploader) ([]string, error) {
	if len(p.Uploads) == 0 {
		return nil, nil
	}
	paths := make([]string, len(p.Uploads))
	for i, up := range p.Uploads {
		paths[i] = up.Path
	}
	resolved, err := u.ResolveLocalImages(paths)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve uploads: %w", err)
	}
	return resolved, nil
}
