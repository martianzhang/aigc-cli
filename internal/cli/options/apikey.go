package options

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// RequireAPIKey returns a clear error when p needs an API key but none is
// configured. Local providers (ollama, local ONNX, loopback endpoints) are
// exempt. providers seeds the actionable choices listed in the error text.
func RequireAPIKey(cmdName string, p *provider.EffectiveProvider, providers map[string]*types.NamedProvider) error {
	if p == nil || !p.RequiresAPIKey() || provider.IsLocalEndpoint(p.BaseURL) {
		return nil
	}

	target := cmdName
	if p.Name != "" {
		target = fmt.Sprintf("provider %q", p.Name)
	}
	base := p.BaseURL
	if base == "" {
		base = types.DefaultAPIBaseURL
	}

	msg := fmt.Sprintf("no API key for %s (resolved base URL: %s)\n", target, base)
	msg += "fix: use --provider <name> or pass --api-key / set OPENAI_API_KEY"
	msg += providerKeyClause(providers)
	msg += "\n" + apiKeyNote(cmdName)
	return errors.New(msg)
}

// providerKeyClause renders the configured providers as an actionable hint.
// Keyed providers are preferred; when none has a key it falls back to listing
// all configured names. Key values are never printed.
func providerKeyClause(providers map[string]*types.NamedProvider) string {
	var keyed, all []string
	for name, np := range providers {
		if np == nil {
			continue
		}
		all = append(all, name)
		if np.APIKey != "" {
			keyed = append(keyed, name)
		}
	}
	if len(keyed) > 0 {
		sort.Strings(keyed)
		return fmt.Sprintf(" (configured with keys: %s)", strings.Join(keyed, ", "))
	}
	if len(all) > 0 {
		sort.Strings(all)
		return fmt.Sprintf(" (configured providers: %s)", strings.Join(all, ", "))
	}
	return ""
}

// apiKeyNote returns the command-specific closing note.
func apiKeyNote(cmdName string) string {
	switch cmdName {
	case "balance":
		return "note: the no-argument form queries every configured provider that has an API key"
	case "task":
		return "note: use the provider whose account submitted the task"
	default:
		return "note: --type and --price listings need no API key"
	}
}
