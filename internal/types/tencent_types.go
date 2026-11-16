package types

// TencentPanoramaResponse is Tencent Cloud TokenHub's synchronous response for
// the Hy-World 360° panorama API (POST /v1/wand/hunyuan-image/world2-panorama).
type TencentPanoramaResponse struct {
	ID        string                `json:"id"`
	Created   int64                 `json:"created"`
	Status    string                `json:"status"`
	Data      []TencentPanoramaData `json:"data"`
	RequestID string                `json:"request_id,omitempty"`
	Usage     *TencentPanoramaUsage `json:"tokenhub_usage,omitempty"`
}

// TencentPanoramaData is one generated panorama. The super-resolution URL is
// the primary output; cover_image_url is a quick-preview rendition.
type TencentPanoramaData struct {
	CoverImageURL           string `json:"cover_image_url,omitempty"`
	SuperResolutionImageURL string `json:"super_resolution_image_url,omitempty"`
}

// TencentPanoramaUsage reports TokenHub billing units for one request.
type TencentPanoramaUsage struct {
	TotalTokens int `json:"total_tokens,omitempty"`
}
