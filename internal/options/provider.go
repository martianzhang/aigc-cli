package options

import (
	"github.com/martianzhang/aigc-cli/internal/provider"
)

// IsAPIMartProvider reports whether p should use APIMart's asynchronous
// task protocol. The --mode flag overrides detection: "async" forces APIMart,
// "sync" disables it. Otherwise the resolved provider's detected vendor
// decides, falling back to base-URL detection when the vendor type is
// uncached.
//
// It reads the provider passed in, not package-global Shared.APIBase, so a
// named per-modality provider is honored from every entrypoint.
func IsAPIMartProvider(p *provider.EffectiveProvider) bool {
	switch Shared.Mode {
	case "async":
		return true
	case "sync":
		return false
	}
	if p != nil && p.ProviderType == provider.APIMart {
		return true
	}
	base := ""
	if p != nil {
		base = p.BaseURL
	}
	return provider.IsAPIMart(base)
}
