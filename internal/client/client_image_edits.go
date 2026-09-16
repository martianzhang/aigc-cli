package client

import (
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// imageEditsRequest is the JSON body accepted by relay panels whose
// image-to-image endpoint is POST /images/edits.
type imageEditsRequest struct {
	Model        string            `json:"model"`
	Prompt       string            `json:"prompt"`
	Size         string            `json:"size,omitempty"`
	N            *int              `json:"n,omitempty"`
	Quality      string            `json:"quality,omitempty"`
	OutputFormat string            `json:"output_format,omitempty"`
	Images       []imageEditsInput `json:"images"`
}

// imageEditsInput is a single entry of the images array.
type imageEditsInput struct {
	ImageURL string `json:"image_url"`
}

// ImageEditsBody builds the JSON body for POST /images/edits. Shared by the
// real request and --dry-run output so both describe the same payload.
func ImageEditsBody(req *types.GenerateRequest) any {
	images := make([]imageEditsInput, 0, len(req.ImageURLs))
	for _, u := range req.ImageURLs {
		images = append(images, imageEditsInput{ImageURL: u})
	}
	return imageEditsRequest{
		Model:        req.Model,
		Prompt:       req.Prompt,
		Size:         req.Size,
		N:            req.N,
		Quality:      req.Quality,
		OutputFormat: req.OutputFormat,
		Images:       images,
	}
}

// ImageGenerateEdits sends an image-to-image request to relay panels that
// accept POST /images/edits with images[].image_url. Image inputs must already
// be usable strings (public URLs or data: URIs); this method never reads files.
func (c *Client) ImageGenerateEdits(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	var result types.OpenAIImageResponse
	if err := c.doJSON(http.MethodPost, imageEditsPath, ImageEditsBody(req), &result); err != nil {
		return nil, err
	}
	return &result, nil
}
