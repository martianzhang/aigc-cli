package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/martianzhang/apimart-cli/internal/ideas"
	"github.com/martianzhang/apimart-cli/internal/service"
)

// outputMarkdown renders ideas markdown to stdout (CLI mode with images & preview).
func outputMarkdown(results []ideas.SearchResult, keywords string, total int, savedFiles []string) error {
	md := ideas.FormatResultsMarkdown(results, keywords, total)
	fmt.Println(md)

	// Local image handling (CLI-specific)
	for _, r := range results {
		if ideasPreview && len(savedFiles) > 0 {
			for range r.Entry.ImageURLs {
				if len(savedFiles) == 0 {
					break
				}
				f := savedFiles[0]
				savedFiles = savedFiles[1:]
				if e := service.PreviewFile(f); e != nil {
					fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
				}
			}
		}
	}
	return nil
}

// outputJSON renders ideas as JSON to stdout.
func outputJSON(results []ideas.SearchResult, total int) error {
	out := struct {
		Total   int               `json:"total"`
		Results []ideas.IdeaEntry `json:"results"`
	}{Total: total}
	for _, r := range results {
		out.Results = append(out.Results, r.Entry)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
