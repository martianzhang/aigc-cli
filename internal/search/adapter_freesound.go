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

const defaultFreesoundBaseURL = "https://freesound.org"

// freesoundFields keeps the response small while including the preview URLs
// that `fetch` needs. Without an explicit fields list Freesound omits previews.
const freesoundFields = "id,name,tags,license,duration,previews,url,filesize,type"

// FreesoundProvider wraps the Freesound APIv2 sound search. It needs an API key
// for searching. Original WAV downloads require OAuth2, but the preview MP3 URLs
// are public, so results point at the preview file for direct `fetch` download.
type FreesoundProvider struct {
	apiKey  string
	baseURL string
}

func NewFreesoundProvider(apiKey string) *FreesoundProvider {
	return &FreesoundProvider{apiKey: apiKey, baseURL: defaultFreesoundBaseURL}
}

func (p *FreesoundProvider) Name() string { return "freesound" }

// MediaTypes marks the provider as a sound-effect source. The list is shown by
// --list-providers; filtering uses the vendor's filter via --inject-json.
func (p *FreesoundProvider) MediaTypes() []string { return []string{"sfx"} }

// Search returns the normalized preview/title list used by chat and MCP.
func (p *FreesoundProvider) Search(query string, limit int) ([]Result, error) {
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

func (p *FreesoundProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	apiURL, err := p.buildURL(query, limit, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("freesound request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("freesound search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("freesound API error %d", resp.StatusCode)
	}

	raw, err := readRawBody(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("freesound read: %w", err)
	}

	var payload struct {
		Results []struct {
			ID       int               `json:"id"`
			Name     string            `json:"name"`
			URL      string            `json:"url"`
			Previews map[string]string `json:"previews"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("freesound parse: %w", err)
	}

	results := make([]Result, 0, len(payload.Results))
	for _, sound := range payload.Results {
		name := strings.TrimSpace(sound.Name)
		if name == "" {
			name = strconv.Itoa(sound.ID)
		}
		results = append(results, Result{URL: freesoundResultURL(sound.ID, sound.URL, sound.Previews), Title: name})
	}
	return results, raw, nil
}

// freesoundResultURL picks the highest-quality public preview, then the sound
// page, so `fetch` always has something it can download.
func freesoundResultURL(id int, pageURL string, previews map[string]string) string {
	for _, key := range []string{"preview-hq-mp3", "preview-lq-mp3", "preview-hq-ogg", "preview-lq-ogg"} {
		if u := strings.TrimSpace(previews[key]); u != "" {
			return u
		}
	}
	if u := strings.TrimSpace(pageURL); u != "" {
		return u
	}
	if id > 0 {
		return fmt.Sprintf("%s/s/%d/", defaultFreesoundBaseURL, id)
	}
	return ""
}

// Preview renders the equivalent Freesound GET with the API key masked.
func (p *FreesoundProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	apiURL, err := p.buildURL(query, limit, opts)
	if err != nil {
		return nil, err
	}
	return &reqbuild.Plan{
		Method: http.MethodGet,
		URL:    apiURL,
		Headers: []reqbuild.Header{
			{Name: "Accept", Value: "application/json"},
			{Name: "Authorization", Value: "Token " + maskedKey(p.apiKey)},
		},
	}, nil
}

// buildURL maps the generic query/limit onto Freesound's search endpoint.
// License and duration filters arrive through --inject-json as the vendor's
// own Solr-style `filter` parameter.
func (p *FreesoundProvider) buildURL(query string, limit int, opts Options) (string, error) {
	u, err := url.Parse(p.baseURL + "/apiv2/search/")
	if err != nil {
		return "", fmt.Errorf("parse freesound base URL: %w", err)
	}
	pageSize := strconv.Itoa(mediaLimit(limit, 1))
	values := u.Query()
	values.Set("query", query)
	values.Set("page_size", pageSize)
	values.Set("fields", freesoundFields)
	u.RawQuery = values.Encode()

	return mergeReservedQuery(u.String(), opts, map[string]string{
		"query":     query,
		"page_size": pageSize,
		"fields":    freesoundFields,
	})
}
