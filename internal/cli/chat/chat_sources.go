package chat

import (
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// sourcesDefaultCap bounds the citation list printed without --verbose, since a
// single MiMo web-search turn can return dozens of URLs.
const sourcesDefaultCap = 10

// printSources lists the citations a provider-side web search attached to the
// assistant message. They go to stderr so streamed/piped stdout stays clean.
// Nothing is printed when the response carries no usable annotations. The list
// is capped unless --verbose is set.
func printSources(result *types.ChatResponse) {
	if result == nil || len(result.Choices) == 0 {
		return
	}
	var lines []string
	seen := map[string]bool{}
	for _, a := range result.Choices[0].Message.Annotations {
		if a.URL == "" || seen[a.URL] {
			continue
		}
		seen[a.URL] = true
		lines = append(lines, annotationLine(a))
	}
	if len(lines) == 0 {
		return
	}

	w := options.Stderr()
	fmt.Fprintf(w, "\n---  Sources (%d):\n", len(lines))

	shown := lines
	if !options.Shared.Verbose && len(shown) > sourcesDefaultCap {
		shown = shown[:sourcesDefaultCap]
	}
	for i, line := range shown {
		fmt.Fprintf(w, "  %d. %s\n", i+1, line)
	}
	if rest := len(lines) - len(shown); rest > 0 {
		fmt.Fprintf(w, "  … and %d more (use -v to list all)\n", rest)
	}
}

// annotationLine renders one citation as "Title (Site) — URL", omitting the
// parts the provider did not supply.
func annotationLine(a types.ChatAnnotation) string {
	label := a.Title
	switch {
	case label == "":
		label = a.SiteName
	case a.SiteName != "" && a.SiteName != label:
		label += " (" + a.SiteName + ")"
	}
	if label == "" {
		return a.URL
	}
	return label + " — " + a.URL
}

// webSearchLine renders MiMo's web_search usage counters, or "" when the
// provider did not report any. Only MiMo serves usage.web_search_usage.
func webSearchLine(p *provider.EffectiveProvider, usage *types.ChatUsage) string {
	if usage == nil || usage.WebSearchUsage == nil {
		return ""
	}
	if p == nil || p.ProviderType != provider.MiMo {
		return ""
	}
	return fmt.Sprintf("Web search: %d call(s), %d page(s)",
		usage.WebSearchUsage.ToolUsage, usage.WebSearchUsage.PageUsage)
}
