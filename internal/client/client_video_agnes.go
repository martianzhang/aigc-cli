package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// agnesSize maps a CLI resolution (480p/720p/1080p) to agnes's accepted size values
// (720P/960P/2K). Unrecognized values are passed through uppercased.
func agnesSize(resolution string) string {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "480p":
		return "720P"
	case "720p":
		return "720P"
	case "1080p", "2k", "4k":
		return "2K"
	default:
		return strings.ToUpper(resolution)
	}
}

// AgnesVideoSubmit sends a video generation request to agnes.ai's POST /v1/videos (async task).
// Body format: {model, mode, prompt, size, aspect_ratio, seconds, n, image?}.
func (c *Client) AgnesVideoSubmit(req *types.VideoGenerateRequest) (*types.AgnesVideoCreateResponse, error) {
	bodyMap := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
		"mode":   "text",
		"n":      1,
	}
	if req.Resolution != "" {
		// agnes 的 size 只接受 "720P"/"960P"/"2K"（不含 480p）。
		// CLI resolution: 480p→720P(最低档), 720p→720P, 1080p→2K, 其他原样大写。
		bodyMap["size"] = agnesSize(req.Resolution)
	}
	if req.Size != "" {
		bodyMap["aspect_ratio"] = req.Size
	}
	if req.Duration != nil && *req.Duration > 0 {
		bodyMap["seconds"] = fmt.Sprintf("%d", *req.Duration)
	}
	if len(req.ImageURLs) > 0 {
		bodyMap["mode"] = "image2video"
		bodyMap["image"] = req.ImageURLs[0]
	} else if len(req.ImageWithRoles) > 0 {
		bodyMap["mode"] = "image2video"
		bodyMap["image"] = req.ImageWithRoles[0].URL
	}

	var result types.AgnesVideoCreateResponse
	if err := c.doJSON(http.MethodPost, agnesVideoSubmitPath, bodyMap, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AgnesVideoQuery polls agnes.ai's video task status via GET /agnesapi?video_id=...&model_name=... .
// 注意：查询端点是域名根路径 /agnesapi（不在 /v1 下），需单独拼 URL。
func (c *Client) AgnesVideoQuery(videoID, modelName string) (*types.AgnesVideoQueryResponse, error) {
	u := strings.TrimRight(c.baseURL, "/")
	// 把 /v1（或任意版本段）去掉，得到域名根，再挂 /agnesapi。
	if i := strings.LastIndex(u, "/"); i > strings.Index(u, "://")+2 {
		u = u[:i]
	}
	path := u + agnesVideoQueryPath + "?video_id=" + url.QueryEscape(videoID) +
		"&model_name=" + url.QueryEscape(modelName)

	var result types.AgnesVideoQueryResponse
	if err := c.doGetAbsolute(path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
