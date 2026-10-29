package provider

import "github.com/martianzhang/aigc-cli/internal/types"

// builtinBaseURLs lists the canonical hosted endpoint of provider types that
// are self-contained protocols. A configured base_url always wins; types not
// listed here (openai, ollama, anthropic, local, …) have no canonical endpoint
// of their own and fall back to the global base_url.
//
// Adding a provider type that ships with a public endpoint is a single line
// here — ResolveCmdProvider stays free of per-type branches.
var builtinBaseURLs = map[types.ProviderType]string{
	types.ProviderSomark: SomarkBaseURL,
}

// builtinBaseURL returns the canonical endpoint for a provider type, or ""
// when the type has none.
func builtinBaseURL(pt types.ProviderType) string {
	return builtinBaseURLs[pt]
}
