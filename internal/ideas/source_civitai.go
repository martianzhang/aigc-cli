package ideas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// civitaiBaseURL is the root of Civitai's public API. It is a var (not a
// const) so tests can point it at an httptest server.
var civitaiBaseURL = "https://civitai.com"

// Civitai has no prompt-search endpoint (GET /api/v1/prompts is a 404, and the
// free-text params on /images are silently ignored). Prompt text exists only as
// items[].meta.prompt inside image results, so a keyword cannot be handed to a
// search API directly. The keyword is approximated in two steps:
//
//  1. /api/v1/models?query=<kw> — Civitai's only full-text search — finds the
//     models (style/subject) that best match the keyword.
//  2. /api/v1/images?modelVersionId=<id>&withMeta=true — pulls each matched
//     model's most-reacted images and extracts their generation prompts.
const (
	// civitaiModelLimit is how many matched models are queried for images.
	civitaiModelLimit = 3
	// civitaiImageLimit bounds how many images are requested per model.
	civitaiImageLimit = 12
)

// civitaiSort is the image ordering; the most-reacted images carry the prompts
// that are most worth reusing.
const civitaiSort = "Most Reactions"

// civitaiSource searches Civitai image prompts.
type civitaiSource struct{}

// Name returns the source name.
func (civitaiSource) Name() string { return SourceCivitai }

// Search resolves the query to models, then collects the prompts of their
// top images. Images without usable prompt metadata are skipped.
func (civitaiSource) Search(ctx context.Context, query string, limit int) ([]IdeaEntry, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, errEmptyQuery
	}

	versions, err := civitaiModelVersions(ctx, trimmed)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, nil
	}

	perModel := limit
	if perModel <= 0 || perModel > civitaiImageLimit {
		perModel = civitaiImageLimit
	}

	// Query every model concurrently; one failing model must not discard the
	// results of the others.
	type outcome struct {
		entries []IdeaEntry
		err     error
	}
	outcomes := make([]outcome, len(versions))
	var wg sync.WaitGroup
	for i, versionID := range versions {
		wg.Add(1)
		go func(i, versionID int) {
			defer wg.Done()
			entries, err := civitaiImages(ctx, versionID, perModel)
			outcomes[i] = outcome{entries: entries, err: err}
		}(i, versionID)
	}
	wg.Wait()

	var entries []IdeaEntry
	var firstErr error
	for _, o := range outcomes {
		if o.err != nil {
			if firstErr == nil {
				firstErr = o.err
			}
			continue
		}
		entries = append(entries, o.entries...)
	}
	// Surface the failure only when nothing at all was collected, so a partial
	// success still returns results (the CLI reports warnings in verbose mode).
	if len(entries) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return truncateEntries(dedupeByPrompt(entries), limit), nil
}

// civitaiModelVersions runs Civitai's full-text model search and returns the
// latest version id of each match (modelVersions are ordered newest-first).
func civitaiModelVersions(ctx context.Context, query string) ([]int, error) {
	endpoint, err := url.Parse(civitaiBaseURL + "/api/v1/models")
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	params := endpoint.Query()
	params.Set("query", query)
	params.Set("limit", strconv.Itoa(civitaiModelLimit))
	endpoint.RawQuery = params.Encode()

	var payload struct {
		Items []struct {
			ModelVersions []struct {
				ID int `json:"id"`
			} `json:"modelVersions"`
		} `json:"items"`
	}
	if err := getJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, err
	}

	versions := make([]int, 0, len(payload.Items))
	for _, item := range payload.Items {
		for _, v := range item.ModelVersions {
			if v.ID != 0 {
				versions = append(versions, v.ID)
				break
			}
		}
	}
	return versions, nil
}

// civitaiImages fetches the most-reacted images of one model version and maps
// the ones carrying prompt metadata to entries.
func civitaiImages(ctx context.Context, modelVersionID, limit int) ([]IdeaEntry, error) {
	endpoint, err := url.Parse(civitaiBaseURL + "/api/v1/images")
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	params := endpoint.Query()
	params.Set("modelVersionId", strconv.Itoa(modelVersionID))
	params.Set("withMeta", "true")
	params.Set("sort", civitaiSort)
	params.Set("nsfw", "None")
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	endpoint.RawQuery = params.Encode()

	var payload struct {
		Items []struct {
			ID       int    `json:"id"`
			URL      string `json:"url"`
			Username string `json:"username"`
			// meta is free-form: it is absent on uploads that shared no
			// metadata, and tools such as ComfyUI/A1111 drop in their own
			// keys. Keep it raw and decode per item so one odd payload cannot
			// fail the whole page.
			Meta json.RawMessage `json:"meta"`
		} `json:"items"`
	}
	if err := getJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, err
	}

	entries := make([]IdeaEntry, 0, len(payload.Items))
	for _, item := range payload.Items {
		prompt := civitaiPrompt(item.Meta)
		if prompt == "" {
			continue
		}
		entry := IdeaEntry{
			Title:  titleFromPrompt(prompt),
			Prompt: prompt,
			Author: item.Username,
			Lang:   "en",
		}
		if item.URL != "" {
			entry.ImageURLs = []string{item.URL}
		}
		if item.ID != 0 {
			entry.SourceURL = civitaiBaseURL + "/images/" + strconv.Itoa(item.ID)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// civitaiPrompt extracts meta.prompt from an image's free-form metadata. It
// returns "" when the metadata is absent, unparseable, or carries no prompt
// text (e.g. a ComfyUI workflow-only upload), so the caller can skip it.
func civitaiPrompt(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var meta struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Prompt)
}

// dedupeByPrompt keeps one entry per distinct prompt, preserving first-seen
// order. Civitai creators commonly post many images generated from the same
// prompt, so keying on the URL would fill the results with identical text.
func dedupeByPrompt(entries []IdeaEntry) []IdeaEntry {
	if len(entries) < 2 {
		return entries
	}
	seen := make(map[string]bool, len(entries))
	unique := entries[:0]
	for _, entry := range entries {
		key := entry.Prompt
		if key == "" {
			key = "url:" + entry.SourceURL
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, entry)
	}
	return unique
}
