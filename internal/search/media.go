package search

import (
	"fmt"
	"net/url"
)

// maxMediaResults caps a single media search. Stock APIs can return hundreds of
// results per page; the CLI is for human/agent-scale discovery, not scraping.
const maxMediaResults = 20

// defaultMediaResults mirrors the CLI's default search limit for direct callers.
const defaultMediaResults = 5

// mediaLimit clamps a requested media result count to a safe range. minimum is
// the vendor's per_page floor (Pixabay requires at least 3).
func mediaLimit(limit, minimum int) int {
	if limit <= 0 {
		limit = defaultMediaResults
	}
	if limit > maxMediaResults {
		limit = maxMediaResults
	}
	if limit < minimum {
		limit = minimum
	}
	return limit
}

// mergeReservedQuery layers --inject-json onto a GET URL, then restores the
// parameters the CLI owns (query text, page size, API key). Vendor-specific
// filters are free to override everything else.
func mergeReservedQuery(rawURL string, opts Options, reserved map[string]string) (string, error) {
	merged, err := injectQuery(rawURL, opts)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(merged)
	if err != nil {
		return "", fmt.Errorf("parse request URL: %w", err)
	}
	values := u.Query()
	for key, value := range reserved {
		values.Set(key, value)
	}
	u.RawQuery = values.Encode()
	return u.String(), nil
}
