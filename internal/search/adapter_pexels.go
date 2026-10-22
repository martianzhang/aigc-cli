package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
)

const defaultPexelsBaseURL = "https://api.pexels.com"

// PexelsProvider wraps Pexels photo and video search. Photos and videos are
// separate endpoints, selected with Options.Type.
type PexelsProvider struct {
	apiKey  string
	baseURL string
}

func NewPexelsProvider(apiKey string) *PexelsProvider {
	return &PexelsProvider{apiKey: apiKey, baseURL: defaultPexelsBaseURL}
}

func (p *PexelsProvider) Name() string { return "pexels" }

// MediaTypes lists the --type values understood by buildURL. Pexels has no
// illustration/vector library: every image is a photo, so `image` is the only
// image type it advertises.
func (p *PexelsProvider) MediaTypes() []string {
	return []string{"image", "video"}
}

// Search returns the normalized page/title list used by chat and MCP.
func (p *PexelsProvider) Search(query string, limit int) ([]Result, error) {
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

func (p *PexelsProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	apiURL, err := p.buildURL(query, limit, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("pexels request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("pexels search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("pexels API error %d", resp.StatusCode)
	}

	raw, err := readRawBody(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("pexels read: %w", err)
	}

	var payload struct {
		Photos []struct {
			URL          string `json:"url"`
			Alt          string `json:"alt"`
			Photographer string `json:"photographer"`
		} `json:"photos"`
		Videos []struct {
			URL  string `json:"url"`
			User struct {
				Name string `json:"name"`
			} `json:"user"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("pexels parse: %w", err)
	}

	results := make([]Result, 0, len(payload.Photos)+len(payload.Videos))
	for _, photo := range payload.Photos {
		title := strings.TrimSpace(photo.Alt)
		if title == "" {
			title = strings.TrimSpace(photo.Photographer)
		}
		results = append(results, Result{URL: photo.URL, Title: title})
	}
	for _, video := range payload.Videos {
		results = append(results, Result{URL: video.URL, Title: strings.TrimSpace(video.User.Name)})
	}
	return results, raw, nil
}

// Preview renders the equivalent Pexels GET with the API key masked.
func (p *PexelsProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	apiURL, err := p.buildURL(query, limit, opts)
	if err != nil {
		return nil, err
	}
	return &reqbuild.Plan{
		Method: http.MethodGet,
		URL:    apiURL,
		Headers: []reqbuild.Header{
			{Name: "Accept", Value: "application/json"},
			{Name: "Authorization", Value: maskedKey(p.apiKey)},
		},
	}, nil
}

// buildURL maps the generic query/limit/type onto Pexels' parameters.
func (p *PexelsProvider) buildURL(query string, limit int, opts Options) (string, error) {
	path, err := pexelsEndpoint(opts.Type)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(p.baseURL + path)
	if err != nil {
		return "", fmt.Errorf("parse pexels base URL: %w", err)
	}
	perPage := mediaLimit(limit, 1)
	values := u.Query()
	values.Set("query", query)
	values.Set("per_page", strconv.Itoa(perPage))
	u.RawQuery = values.Encode()

	return mergeReservedQuery(u.String(), opts, map[string]string{
		"query":    query,
		"per_page": strconv.Itoa(perPage),
	})
}

// pexelsEndpoint returns the API path for a media type.
func pexelsEndpoint(mediaType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "", "image":
		return "/v1/search", nil
	case "video":
		return "/videos/search", nil
	case "photo":
		return "", fmt.Errorf(
			"pexels uses --type image for photos (video is separate); illustration/vector are unavailable",
		)
	default:
		return "", fmt.Errorf(
			"pexels supports --type image|video, got %q",
			mediaType,
		)
	}
}
