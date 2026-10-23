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

const defaultSFXMintBaseURL = "https://sfxmint.com"

// SFXMintProvider wraps SFXMint's free sound-effect library. Searching and
// downloading need no API key, and every result carries a permanent WAV/MP3
// URL, so the raw JSON feeds `fetch` directly. All library sounds are CC0.
type SFXMintProvider struct {
	baseURL string
}

func NewSFXMintProvider() *SFXMintProvider {
	return &SFXMintProvider{baseURL: defaultSFXMintBaseURL}
}

func (p *SFXMintProvider) Name() string { return "sfxmint" }

// MediaTypes marks the provider as a sound-effect source. The list is shown by
// --list-providers; filtering uses --inject-json, not --type.
func (p *SFXMintProvider) MediaTypes() []string { return []string{"sfx"} }

// sfxMintSound is the subset of a search hit needed for the normalized list.
// The raw vendor JSON is returned untouched; this only feeds chat/MCP results.
type sfxMintSound struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	PageURL string `json:"page_url"`
	MP3URL  string `json:"mp3_url"`
	WAVURL  string `json:"wav_url"`
}

// Search returns the normalized audio/title list used by chat and MCP.
func (p *SFXMintProvider) Search(query string, limit int) ([]Result, error) {
	results, _, err := p.SearchRaw(query, limit, Options{})
	return results, err
}

func (p *SFXMintProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	apiURL, err := p.buildURL(query, limit, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("sfxmint request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("sfxmint search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("sfxmint API error %d", resp.StatusCode)
	}

	raw, err := readRawBody(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("sfxmint read: %w", err)
	}

	sounds, err := parseSFXMintSounds(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("sfxmint parse: %w", err)
	}

	results := make([]Result, 0, len(sounds))
	for _, sound := range sounds {
		results = append(results, Result{URL: sfxMintResultURL(sound), Title: sfxMintTitle(sound)})
	}
	return results, raw, nil
}

// parseSFXMintSounds accepts both response shapes: the legacy top-level array
// and the structured object (`response=structured`) whose hits live under
// `candidates`. Callers reach the second one via --inject-json.
func parseSFXMintSounds(raw json.RawMessage) ([]sfxMintSound, error) {
	var sounds []sfxMintSound
	if err := json.Unmarshal(raw, &sounds); err == nil {
		return sounds, nil
	}

	var structured struct {
		Candidates []sfxMintSound `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &structured); err != nil {
		return nil, err
	}
	return structured.Candidates, nil
}

// sfxMintResultURL prefers the WAV download (highest quality and what `fetch`
// should save), then MP3, then the landing page.
func sfxMintResultURL(sound sfxMintSound) string {
	for _, candidate := range []string{sound.WAVURL, sound.MP3URL, sound.PageURL} {
		if u := strings.TrimSpace(candidate); u != "" {
			return u
		}
	}
	if slug := strings.TrimSpace(sound.Slug); slug != "" {
		return defaultSFXMintBaseURL + "/sounds/" + slug
	}
	return ""
}

func sfxMintTitle(sound sfxMintSound) string {
	if title := strings.TrimSpace(sound.Title); title != "" {
		return title
	}
	return strings.TrimSpace(sound.Slug)
}

// Preview renders the equivalent SFXMint GET. The provider needs no key, so
// there is nothing to mask.
func (p *SFXMintProvider) Preview(query string, limit int, opts Options) (*reqbuild.Plan, error) {
	apiURL, err := p.buildURL(query, limit, opts)
	if err != nil {
		return nil, err
	}
	return &reqbuild.Plan{
		Method: http.MethodGet,
		URL:    apiURL,
		Headers: []reqbuild.Header{
			{Name: "Accept", Value: "application/json"},
		},
	}, nil
}

// buildURL maps the generic query/limit onto SFXMint's search endpoint. Vendor
// filters (max_duration_ms, loop, format, category) arrive via --inject-json.
func (p *SFXMintProvider) buildURL(query string, limit int, opts Options) (string, error) {
	u, err := url.Parse(p.baseURL + "/api/v1/search")
	if err != nil {
		return "", fmt.Errorf("parse sfxmint base URL: %w", err)
	}
	pageSize := strconv.Itoa(mediaLimit(limit, 1))
	values := u.Query()
	values.Set("q", query)
	values.Set("limit", pageSize)
	u.RawQuery = values.Encode()

	return mergeReservedQuery(u.String(), opts, map[string]string{
		"q":     query,
		"limit": pageSize,
	})
}
