package ideas

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// promptsChatBaseURL is the root of the prompts.chat API.
// It is a var (not a const) so tests can point it at an httptest server.
var promptsChatBaseURL = "https://prompts.chat"

// promptsChatSource searches the prompts.chat community prompt library.
type promptsChatSource struct{}

// Name returns the source name.
func (promptsChatSource) Name() string { return SourcePromptsChat }

// Search fetches matching prompts from prompts.chat.
func (promptsChatSource) Search(ctx context.Context, query string, limit int) ([]IdeaEntry, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, errEmptyQuery
	}

	endpoint, err := url.Parse(promptsChatBaseURL + "/api/prompts")
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	params := endpoint.Query()
	params.Set("q", trimmed)
	if limit > 0 {
		params.Set("perPage", strconv.Itoa(limit))
	}
	endpoint.RawQuery = params.Encode()

	var payload struct {
		Prompts []struct {
			Title    string `json:"title"`
			Slug     string `json:"slug"`
			Content  string `json:"content"`
			MediaURL string `json:"mediaUrl"`
			Author   struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"prompts"`
	}
	if err := getJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, err
	}

	entries := make([]IdeaEntry, 0, len(payload.Prompts))
	for _, item := range payload.Prompts {
		entry := IdeaEntry{
			Title:  item.Title,
			Prompt: item.Content,
			Author: item.Author.Name,
			Lang:   "en",
		}
		if item.Slug != "" {
			entry.SourceURL = promptsChatBaseURL + "/prompts/" + item.Slug
		}
		if item.MediaURL != "" {
			entry.ImageURLs = []string{item.MediaURL}
		}
		entries = append(entries, entry)
	}
	return truncateEntries(entries, limit), nil
}
