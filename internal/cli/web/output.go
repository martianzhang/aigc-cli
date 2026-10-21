package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/martianzhang/aigc-cli/internal/search"
)

// fallbackResult is used only when a provider cannot expose its raw body, so
// even test doubles produce the same lowercase title/url shape.
type fallbackResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// writeRawJSON prints the winning provider's response body verbatim, pretty-
// printed when it is valid JSON. Providers that do not expose a raw response
// (test doubles) fall back to the normalized result list so the output shape
// stays valid JSON.
func writeRawJSON(w io.Writer, result *search.SearchResult) error {
	if result == nil {
		_, err := io.WriteString(w, "null\n")
		return err
	}

	raw := result.Raw
	if len(raw) == 0 {
		fallback := make([]fallbackResult, 0, len(result.Results))
		for _, r := range result.Results {
			fallback = append(fallback, fallbackResult{Title: r.Title, URL: r.URL})
		}
		data, err := json.Marshal(fallback)
		if err != nil {
			return fmt.Errorf("encode search results: %w", err)
		}
		raw = data
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err == nil {
		pretty.WriteByte('\n')
		_, err = w.Write(pretty.Bytes())
		return err
	}

	// Not valid JSON (e.g. a provider error page returned with HTTP 200):
	// print it unchanged rather than hiding the debugging payload.
	if _, err := w.Write(raw); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}
