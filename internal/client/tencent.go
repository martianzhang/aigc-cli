package client

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/martianzhang/aigc-cli/internal/httpx"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	// TencentPanoramaPath is relative to the normalized base URL, which already
	// carries /v1: <base>/wand/hunyuan-image/world2-panorama.
	TencentPanoramaPath = "/wand/hunyuan-image/world2-panorama"
	// TencentPanoramaModel is TokenHub's model id for HY-World 2.1 Panorama.
	TencentPanoramaModel = "hy-world2-panorama"
	// tencentPanoramaPromptLimit is Tencent's documented 600-character cap.
	tencentPanoramaPromptLimit = 600
)

// tencentPanoramaRequest is TokenHub's native panorama body. It accepts no
// size/quality fields: the model always returns one equirectangular panorama.
type tencentPanoramaRequest struct {
	Model  string   `json:"model"`
	Prompt string   `json:"prompt"`
	Images []string `json:"images,omitempty"`
}

// TencentPanoramaBody builds the request body for Tencent Cloud TokenHub's
// Hy-World panorama API. Unlike OpenAI-compatible endpoints it takes public
// image URLs (never local paths or data URIs) for image-to-panorama.
func TencentPanoramaBody(req *types.GenerateRequest) (any, error) {
	if len(req.RawJSON) > 0 {
		return req.BodyOrRaw(nil)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = TencentPanoramaModel
	}
	if n := utf8.RuneCountInString(req.Prompt); n > tencentPanoramaPromptLimit {
		return nil, fmt.Errorf("prompt is %d characters; Tencent panorama accepts at most %d", n, tencentPanoramaPromptLimit)
	}
	for _, image := range req.ImageURLs {
		if !isPublicHTTPURL(image) {
			return nil, fmt.Errorf("tencent panorama needs a public image URL for --image-url, got %q (local files and data URIs are not accepted)", image)
		}
	}
	return req.BodyOrRaw(tencentPanoramaRequest{
		Model:  model,
		Prompt: req.Prompt,
		Images: req.ImageURLs,
	})
}

// isPublicHTTPURL reports whether raw is an absolute http(s) URL, the only
// image source TokenHub accepts.
func isPublicHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// TencentPanoramaGenerate calls TokenHub's synchronous panorama API and maps
// its URL-based response onto the shared OpenAI-style image shape, so the
// existing download/save path works unchanged.
func (c *Client) TencentPanoramaGenerate(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	body, err := TencentPanoramaBody(req)
	if err != nil {
		return nil, err
	}
	var raw types.TencentPanoramaResponse
	if err := c.doJSONLimit(TencentPanoramaPath, body, &raw, httpx.ImageBodyLimit()); err != nil {
		return nil, err
	}
	if raw.Status != "" && raw.Status != "succeeded" {
		return nil, fmt.Errorf("tencent panorama generation %s", raw.Status)
	}

	out := &types.OpenAIImageResponse{Created: raw.Created}
	for _, item := range raw.Data {
		imageURL := item.SuperResolutionImageURL
		if imageURL == "" {
			imageURL = item.CoverImageURL
		}
		if imageURL != "" {
			out.Data = append(out.Data, types.OpenAIImageData{URL: imageURL})
		}
	}
	if raw.Usage != nil {
		out.Usage = &types.OpenAIImageUsage{TotalTokens: raw.Usage.TotalTokens}
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("tencent panorama returned no images")
	}
	return out, nil
}
