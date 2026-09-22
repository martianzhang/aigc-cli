package ideas

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// aipromptLibraryBaseURL is the root of the aipromptslibrary.sh API.
// It is a var (not a const) so tests can point it at an httptest server.
var aipromptLibraryBaseURL = "https://aipromptslibrary.sh"

// aiPromptLibrarySource searches the aipromptslibrary.sh image-generation prompts.
type aiPromptLibrarySource struct{}

// Name returns the source name.
func (aiPromptLibrarySource) Name() string { return SourceAIPromptLibrary }

// Search fetches matching image-generation prompts from aipromptslibrary.sh.
func (aiPromptLibrarySource) Search(ctx context.Context, query string, limit int) ([]IdeaEntry, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, errEmptyQuery
	}

	endpoint, err := url.Parse(aipromptLibraryBaseURL + "/api/prompts")
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	params := endpoint.Query()
	params.Set("q", trimmed)
	params.Set("category", "image_generation")
	params.Set("include_prompt_text", "true")
	if limit > 0 {
		params.Set("pageSize", strconv.Itoa(limit))
	}
	endpoint.RawQuery = params.Encode()

	var payload struct {
		Items []struct {
			Title  string `json:"title"`
			URL    string `json:"url"`
			Prompt string `json:"prompt"`
		} `json:"items"`
	}
	if err := getJSON(ctx, endpoint.String(), &payload); err != nil {
		return nil, err
	}

	entries := make([]IdeaEntry, 0, len(payload.Items))
	for _, item := range payload.Items {
		entries = append(entries, IdeaEntry{
			Title:     item.Title,
			Prompt:    item.Prompt,
			SourceURL: item.URL,
			Lang:      "en",
		})
	}
	return truncateEntries(entries, limit), nil
}
