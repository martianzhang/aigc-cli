package ideas

import (
	"fmt"
	"strings"
)

// FormatResultsMarkdown renders ideas as markdown string.
func FormatResultsMarkdown(results []SearchResult, keywords string, total int) string {
	var b strings.Builder
	header := fmt.Sprintf("Found %d result(s) for \"%s\"", total, keywords)
	if total > len(results) {
		header += fmt.Sprintf(" (showing %d/%d)", len(results), total)
	}
	fmt.Fprintf(&b, "%s\n\n", header)

	for i, r := range results {
		e := r.Entry
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
