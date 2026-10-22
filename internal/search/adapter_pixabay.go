package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
)

const defaultPixabayBaseURL = "https://pixabay.com"

// pixabayCacheTTL follows Pixabay's API requirement: responses must be cached
// for 24 hours.
const pixabayCacheTTL = 24 * time.Hour

// PixabayProvider wraps Pixabay's image and video search APIs. The image API
// covers photos, illustrations and vectors via image_type; video is a separate
// endpoint (selected with Options.Type).
type PixabayProvider struct {
	apiKey  string
	baseURL string
}

func NewPixabayProvider(apiKey string) *PixabayProvider {
	return &PixabayProvider{apiKey: apiKey, baseURL: defaultPixabayBaseURL}
}

func (p *PixabayProvider) Name() string { return "pixabay" }

// MediaTypes lists the --type values understood by BuildURL.
func (p *PixabayProvider) MediaTypes() []string {
	return []string{"image", "photo", "illustration", "vector", "video"}
}

// Search returns the normalized page/title list used by chat and MCP. The CLI
// prints SearchRaw's untouched vendor JSON instead.
func (p *PixabayProvider) Search(query string, limit int) ([]Result, error) {
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

func (p *PixabayProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	apiURL, err := p.buildURL(query, limit, opts)
	if err != nil {
		return nil, nil, err
	}

	key := cacheKey(apiURL)
	var raw json.RawMessage
	cached := false
	if !opts.NoCache {
		raw, cached = readSearchCache("pixabay", key, pixabayCacheTTL)
	}
	if !cached {
		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("pixabay request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("pixabay search: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return nil, nil, fmt.Errorf("pixabay API error %d", resp.StatusCode)
		}

		raw, err = readRawBody(resp.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("pixabay read: %w", err)
		}
		writeSearchCache("pixabay", key, raw)
	}

	var payload struct {
		Hits []struct {
			PageURL string `json:"pageURL"`
			Tags    string `json:"tags"`
			User    string `json:"user"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("pixabay parse: %w", err)
	}

	results := make([]Result, 0, len(payload.Hits))
	for _, hit := range payload.Hits {
		title := strings.TrimSpace(hit.Tags)
		if title == "" {
			title = strings.TrimSpace(hit.User)
		}
		results = append(results, Result{URL: hit.PageURL, Title: title})
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, raw, nil
}

// Preview renders the equivalent Pixabay GET with the API key masked.
func (p *PixabayProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	apiURL, err := p.buildURL(query, limit, opts)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("parse pixabay URL: %w", err)
	}
	values := u.Query()
	values.Set("key", maskedKey(p.apiKey))
	u.RawQuery = values.Encode()

	return &reqbuild.Plan{
		Method: http.MethodGet,
		URL:    u.String(),
		Headers: []reqbuild.Header{
			{Name: "Accept", Value: "application/json"},
		},
	}, nil
}

// buildURL maps the generic query/limit/type onto Pixabay's parameters.
func (p *PixabayProvider) buildURL(query string, limit int, opts Options) (string, error) {
	path, imageType, err := pixabayEndpoint(opts.Type)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(p.baseURL + path)
	if err != nil {
		return "", fmt.Errorf("parse pixabay base URL: %w", err)
	}
	perPage := mediaLimit(limit, 3)
	values := u.Query()
	values.Set("key", p.apiKey)
	values.Set("q", query)
	values.Set("per_page", strconv.Itoa(perPage))
	if imageType != "" {
		values.Set("image_type", imageType)
	}
	u.RawQuery = values.Encode()

	reserved := map[string]string{
		"key":      p.apiKey,
		"q":        query,
		"per_page": strconv.Itoa(perPage),
	}
	if imageType != "" {
		reserved["image_type"] = imageType
	}
	return mergeReservedQuery(u.String(), opts, reserved)
}

// pixabayEndpoint returns the API path and image_type for a media type.
func pixabayEndpoint(mediaType string) (path, imageType string, err error) {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "":
		return "/api/", "", nil
	case "image":
		return "/api/", "all", nil
	case "photo":
		return "/api/", "photo", nil
	case "illustration":
		return "/api/", "illustration", nil
	case "vector":
		return "/api/", "vector", nil
	case "video":
		return "/api/videos/", "", nil
	default:
		return "", "", fmt.Errorf(
			"pixabay supports --type image|photo|illustration|vector|video, got %q",
			mediaType,
		)
	}
}
