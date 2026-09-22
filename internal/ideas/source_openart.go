package ideas

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// openArtBaseURL is the root of the openart.ai API.
// It is a var (not a const) so tests can point it at an httptest server.
var openArtBaseURL = "https://openart.ai"

// openArtTitleMaxRunes bounds the derived title length.
const openArtTitleMaxRunes = 60

// openArtSource searches openart.ai community prompts. EXPERIMENTAL: the
// endpoint is undocumented, has no result-limit parameter, and may change.
type openArtSource struct{}

// Name returns the source name.
func (openArtSource) Name() string { return SourceOpenArt }

// Search fetches matching prompts from openart.ai and truncates the result.
func (openArtSource) Search(ctx context.Context, query string, limit int) ([]IdeaEntry, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, errEmptyQuery
	}

	endpoint, err := url.Parse(openArtBaseURL + "/api/search")
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	params := endpoint.Query()
	params.Set("q", trimmed)
	endpoint.RawQuery = params.Encode()

	var payload struct {
		Items []struct {
			Prompt   string `json:"prompt"`
			ImageURL string `json:"image_url"`
			Profile  struct {
				Name string `json:"name"`
			} `json:"userProfile"`
		} `json:"items"`
	}
	if err := getJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, err
	}

	entries := make([]IdeaEntry, 0, len(payload.Items))
	for _, item := range payload.Items {
		// The search feed contains image-only items with no prompt text; they
		// have no URL and no derivable title, so they cannot serve as prompts.
		if strings.TrimSpace(item.Prompt) == "" {
			continue
		}
		entry := IdeaEntry{
			Title:  titleFromPrompt(item.Prompt),
			Prompt: item.Prompt,
			Author: item.Profile.Name,
			Lang:   "en",
		}
		if item.ImageURL != "" {
			entry.ImageURLs = []string{item.ImageURL}
		}
		entries = append(entries, entry)
	}
	return truncateEntries(entries, limit), nil
}

// titleFromPrompt derives a short label from the first line of a prompt.
func titleFromPrompt(prompt string) string {
	line := strings.TrimSpace(prompt)
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	runes := []rune(line)
	if len(runes) > openArtTitleMaxRunes {
		return string(runes[:openArtTitleMaxRunes]) + "…"
	}
	return line
}
