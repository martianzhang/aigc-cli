package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/martianzhang/apimart-cli/internal/service"
)

// --- markdown output ---

// formatResultsMarkdown renders ideas as markdown string.
// Shared by CLI output and agent tool results.
func formatResultsMarkdown(results []searchResult, keywords string, total int) string {
	var b strings.Builder
	header := fmt.Sprintf("Found %d result(s) for \"%s\"", total, keywords)
	if total > len(results) {
		header += fmt.Sprintf(" (showing %d/%d)", len(results), total)
	}
	fmt.Fprintf(&b, "%s\n\n", header)

	for i, r := range results {
		e := r.entry
		if i > 0 {
			fmt.Fprintf(&b, "---\n\n")
		}
		title := e.Title
		if title == "" {
			title = fmt.Sprintf("Result %d", i+1)
		}
		fmt.Fprintf(&b, "## %s\n\n", title)

		prompt := e.Prompt
		if e.Lang == "zh" && e.PromptZh != "" {
			prompt = e.PromptZh
		}
		fmt.Fprintf(&b, "```\n%s\n```\n\n", prompt)

		for _, u := range e.ImageURLs {
			fmt.Fprintf(&b, "![ref](%s)\n\n", u)
		}

		var meta []string
		if e.Author != "" {
			meta = append(meta, "Author: "+e.Author)
		}
		if e.SourceURL != "" {
			meta = append(meta, "[Source]("+e.SourceURL+")")
		}
		if e.License != "" {
			meta = append(meta, e.License)
		}
		if len(meta) > 0 {
			fmt.Fprintf(&b, "%s\n", strings.Join(meta, " · "))
		}
	}
	return b.String()
}

// outputMarkdown renders ideas markdown to stdout (CLI mode with images & preview).
func outputMarkdown(results []searchResult, keywords string, total int, savedFiles []string) error {
	md := formatResultsMarkdown(results, keywords, total)
	fmt.Println(md)

	// Local image handling (CLI-specific)
	for _, r := range results {
		// Inline preview: show saved images right after their entry
		if ideasPreview && len(savedFiles) > 0 {
			for range r.entry.ImageURLs {
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

// --- json output ---

func outputJSON(results []searchResult, total int) error {
	out := struct {
		Total   int         `json:"total"`
		Results []IdeaEntry `json:"results"`
	}{Total: total}
	for _, r := range results {
		out.Results = append(out.Results, r.entry)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
