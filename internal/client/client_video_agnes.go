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
// See https://agnes-ai.com/zh-Hans/docs/agnes-video-25-flash for the official API spec.
func (c *Client) AgnesVideoSubmit(req *types.VideoGenerateRequest) (*types.AgnesVideoCreateResponse, error) {
	bodyMap := map[string]interface{}{
		"model":  req.Model,
		"prompt": req.Prompt,
		"mode":   "text",
		"n":      1,
	}
	// size: Flash 固定 "720P"。CLI resolution 默认 480p 经 agnesSize 映射为 720P。
	if req.Resolution != "" {
		bodyMap["size"] = agnesSize(req.Resolution)
	}
	// aspect_ratio: CLI 的 Size 是宽高比（16:9 等），agnès 用 aspect_ratio。
	if req.Size != "" {
		bodyMap["aspect_ratio"] = req.Size
	}
	if req.Duration != nil && *req.Duration > 0 {
		bodyMap["seconds"] = fmt.Sprintf("%d", *req.Duration)
	}

	// 媒体输入：agnès 三种模式互斥。keyframe（首尾帧）优先于 reference（图片参考）。
	switch {
	case len(req.ImageWithRoles) > 0:
		bodyMap["mode"] = "keyframe"
		for _, r := range req.ImageWithRoles {
			switch r.Role {
			case "first_frame":
				bodyMap["first_frame"] = r.URL
			case "last_frame":
				bodyMap["last_frame"] = r.URL
			}
		}
	case len(req.ImageURLs) > 0:
		bodyMap["mode"] = "reference"
		bodyMap["images"] = req.ImageURLs
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
