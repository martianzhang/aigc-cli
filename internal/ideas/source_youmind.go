package ideas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// youmindBaseURL is the site root. It is a var so tests can point it at an
// httptest server.
var youmindBaseURL = "https://youmind.com"

// youmindQueries are the public prompt endpoints queried per search. The image
// endpoint requires a model, so the two largest general-purpose libraries are
// combined; the video endpoint searches every video model when model is
// omitted.
var youmindQueries = []struct {
	path  string
	model string
}{
	{path: "/youmarketing-api/prompts", model: "gpt-image-2"},
	{path: "/youmarketing-api/prompts", model: "gpt-image-2.5"},
	{path: "/youmarketing-api/video-prompts"},
	{path: "/youmarketing-api/prompts", model: "gpt-6-astra"},
}

// youmindSource searches YouMind's public prompt library through the same
// JSON API the website uses. No API key is required.
type youmindSource struct{}

// Name returns the source name.
func (youmindSource) Name() string { return SourceYouMind }

type youmindItem struct {
	ID                int    `json:"id"`
	Title             string `json:"title"`
	Slug              string `json:"slug"`
	Content           string `json:"content"`
	TranslatedContent string `json:"translatedContent"`
	Language          string `json:"language"`
	Author            struct {
		Name string `json:"name"`
	} `json:"author"`
	Media  []string `json:"media"`
	Videos []struct {
		Thumbnail string `json:"thumbnail"`
	} `json:"videos"`
}

// Search queries the image, video and webpage prompt endpoints concurrently
// and merges the results round-robin so no single media type dominates.
func (youmindSource) Search(ctx context.Context, query string, limit int) ([]IdeaEntry, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, errEmptyQuery
	}

	type response struct {
		entries []IdeaEntry
		err     error
	}
	responses := make([]response, len(youmindQueries))
	var wg sync.WaitGroup
	for i, q := range youmindQueries {
		wg.Add(1)
		go func(i int, path, model string) {
			defer wg.Done()
			entries, err := youmindFetch(ctx, path, model, trimmed, limit)
			responses[i] = response{entries: entries, err: err}
		}(i, q.path, q.model)
	}
	wg.Wait()

	var (
		firstErr error
		lists    [][]IdeaEntry
	)
	for _, r := range responses {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		lists = append(lists, r.entries)
	}

	merged := interleaveEntries(lists, limit)
	if len(merged) == 0 {
		return nil, firstErr
	}
	return merged, nil
}

// youmindFetch performs one prompt search and maps its items to IdeaEntry.
func youmindFetch(ctx context.Context, path, model, query string, limit int) ([]IdeaEntry, error) {
	payload := map[string]any{
		"q":     query,
		"page":  1,
		"limit": limit,
	}
	if model != "" {
		payload["model"] = model
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("youmind marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, youmindBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("youmind request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The public endpoint rejects requests without a same-site Origin/Referer.
	req.Header.Set("Origin", youmindBaseURL)
	req.Header.Set("Referer", youmindBaseURL+"/prompts")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; aigc-cli)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("youmind request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var envelope struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&envelope)
		if msg := strings.TrimSpace(envelope.Error); msg != "" {
			return nil, fmt.Errorf("youmind API error %d: %s", resp.StatusCode, msg)
		}
		return nil, fmt.Errorf("youmind API error %d", resp.StatusCode)
	}

	var out struct {
		Prompts []youmindItem `json:"prompts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("youmind parse: %w", err)
	}

	detailPath := "/prompts/"
	if path == "/youmarketing-api/video-prompts" {
		detailPath = "/video-prompts/"
	}
	entries := make([]IdeaEntry, 0, len(out.Prompts))
	for _, item := range out.Prompts {
		prompt := strings.TrimSpace(item.Content)
		if prompt == "" {
			prompt = strings.TrimSpace(item.TranslatedContent)
		}
		if prompt == "" {
			continue
		}
		entry := IdeaEntry{
			Title:  item.Title,
			Prompt: prompt,
			Author: item.Author.Name,
			Lang:   item.Language,
		}
		if item.Slug != "" {
			entry.SourceURL = youmindBaseURL + detailPath + item.Slug
		}
		entry.ImageURLs = append(entry.ImageURLs, item.Media...)
		for _, v := range item.Videos {
			if v.Thumbnail != "" {
				entry.ImageURLs = append(entry.ImageURLs, v.Thumbnail)
			}
		}
		entries = append(entries, entry)
	}
	return truncateEntries(entries, limit), nil
}

// interleaveEntries merges per-endpoint lists round-robin, de-duplicating by
// source URL (falling back to title+prompt) and capping at limit.
func interleaveEntries(lists [][]IdeaEntry, limit int) []IdeaEntry {
	seen := make(map[string]bool)
	merged := make([]IdeaEntry, 0)
	for i := 0; ; i++ {
		added := false
		for _, list := range lists {
			if i >= len(list) {
				continue
			}
			added = true
			entry := list[i]
			key := entry.SourceURL
			if key == "" {
				key = entry.Title + "\x00" + entry.Prompt
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, entry)
			if limit > 0 && len(merged) >= limit {
				return merged
			}
		}
		if !added {
			return merged
		}
	}
}
