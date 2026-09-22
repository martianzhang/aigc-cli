package ideas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// getJSON performs a GET request and decodes the JSON body into out.
// It uses http.DefaultClient so the globally configured proxy applies.
func getJSON(ctx context.Context, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("cannot build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("invalid response body: %w", err)
	}
	return nil
}

// truncateEntries caps entries at limit when limit is positive.
func truncateEntries(entries []IdeaEntry, limit int) []IdeaEntry {
	if limit > 0 && len(entries) > limit {
		return entries[:limit]
	}
	return entries
}
