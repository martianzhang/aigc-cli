package web

import (
	"strings"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// resolveProvider applies the CLI > --json > defaults.search.provider >
// duckduckgo precedence. It deliberately ignores the LLM provider (--provider
// is shadowed by the search command's own flag).
func resolveProvider(cliValue string) string {
	if p := strings.TrimSpace(cliValue); p != "" {
		return p
	}
	if p := options.SearchProviderDefault(); p != "" {
		return p
	}
	return "duckduckgo"
}
