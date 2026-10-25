package chat

import (
	"fmt"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// sourcesDefaultCap bounds the citation list printed without --verbose, since a
// single MiMo web-search turn can return dozens of URLs.
const sourcesDefaultCap = 10

// collectSources returns the usable citations attached to the first choice,
// de-duplicated by URL and preserving provider order.
func collectSources(result *types.ChatResponse) []types.ChatAnnotation {
	if result == nil || len(result.Choices) == 0 {
		return nil
	}
	var out []types.ChatAnnotation
	seen := map[string]bool{}
	for _, a := range result.Choices[0].Message.Annotations {
		if a.URL == "" || seen[a.URL] {
			continue
		}
		seen[a.URL] = true
		out = append(out, a)
	}
	return out
}

// printSources lists the citations a provider-side web search attached to the
// assistant message. They go to stderr so streamed/piped stdout stays clean.
// Nothing is printed when the response carries no usable annotations. The list
// is capped unless --verbose is set.
func printSources(result *types.ChatResponse) {
	sources := collectSources(result)
	if len(sources) == 0 {
		return
	}

	w := options.Stderr()
	fmt.Fprintf(w, "\n---  Sources (%d):\n", len(sources))

	shown := sources
	if !options.Shared.Verbose && len(shown) > sourcesDefaultCap {
		shown = shown[:sourcesDefaultCap]
	}
	for i, a := range shown {
		fmt.Fprintf(w, "  %d. %s\n", i+1, annotationLine(a))
	}
	if rest := len(sources) - len(shown); rest > 0 {
		fmt.Fprintf(w, "  … and %d more (use -v to list all)\n", rest)
	}
}

// formatSources renders citations for the TUI message area. URLs become OSC 8
// hyperlinks (ignored by terminals that do not support them). The list is
// capped unless verbose is set, matching printSources.
func formatSources(sources []types.ChatAnnotation, verbose bool) string {
	if len(sources) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sources (%d):", len(sources))

	shown := sources
	if !verbose && len(shown) > sourcesDefaultCap {
		shown = shown[:sourcesDefaultCap]
	}
	for i, a := range shown {
		fmt.Fprintf(&b, "\n  %d. %s", i+1, annotationHyperlink(a))
	}
	if rest := len(sources) - len(shown); rest > 0 {
		fmt.Fprintf(&b, "\n  … and %d more (use -v to list all)", rest)
	}
	return b.String()
}

// annotationHyperlink renders one citation, wrapping only the URL in an OSC 8
// hyperlink so the title stays readable and selectable.
func annotationHyperlink(a types.ChatAnnotation) string {
	line := annotationLine(a)
	if a.URL == "" {
		return line
	}
	if prefix := strings.TrimSuffix(line, a.URL); prefix != line {
		return prefix + osc8(a.URL, a.URL)
	}
	return osc8(a.URL, line)
}

// osc8 wraps text in an OSC 8 hyperlink targeting url.
func osc8(url, text string) string {
	if url == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
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
