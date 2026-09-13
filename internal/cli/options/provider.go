package options

import "github.com/martianzhang/aigc-cli/internal/provider"

// IsOpenRouterProvider reports whether the current base URL points to OpenRouter.
func IsOpenRouterProvider() bool {
	return provider.IsOpenRouter(Shared.APIBase)
}

// IsAPIMartProvider reports whether to use APIMart async mode.
func IsAPIMartProvider() bool {
	switch Shared.Mode {
	case "async":
		return true
	case "sync":
		return false
	default:
		base := Shared.APIBase
		if base == "" {
			base = DefaultBaseURL
		}
		return provider.IsAPIMart(base)
	}
}
