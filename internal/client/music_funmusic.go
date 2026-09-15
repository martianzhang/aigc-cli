package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// funMusicPath is the DashScope-native Fun-Music path. It is not part of the
// OpenAI-compatible /v1 surface, so requests bypass c.baseURL.
const funMusicPath = "/api/v1/services/audio/music/generation"

// FunMusicEndpoint derives the DashScope-native Fun-Music endpoint from a
// provider base URL. Only scheme and host are used: chat providers are
// configured with the compatible-mode path (…/compatible-mode/v1), which is
// replaced by the native services path. A host-only base URL also works.
func FunMusicEndpoint(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" {
		return "https://dashscope.aliyuncs.com" + funMusicPath
	}
	return u.Scheme + "://" + u.Host + funMusicPath
}

// FunMusicGenerate calls the synchronous DashScope-native Fun-Music endpoint,
// which returns the finished audio URL in the same response. body is taken as a
// map so the --json overlay can pass through vendor-specific fields (gender,
// enable_aigc_watermark) that are not modelled as typed fields.
func (c *Client) FunMusicGenerate(body map[string]any) (*types.FunMusicResponse, error) {
	var result types.FunMusicResponse
	if err := c.doJSONAbsolute(http.MethodPost, FunMusicEndpoint(c.baseURL), body, &result, nil); err != nil {
		return nil, err
	}
	if result.Code != "" {
		msg := result.Message
		if msg == "" {
			msg = "unknown error"
		}
		return nil, fmt.Errorf("fun-music error %s: %s", result.Code, msg)
	}
	if result.Output.Audio.URL == "" {
		return nil, fmt.Errorf("fun-music returned no audio url")
	}
	return &result, nil
}
