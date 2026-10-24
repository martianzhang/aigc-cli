package models

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// providerRow is the credential-free view of one configured provider.
type providerRow struct {
	name string
	typ  string
	host string
	key  string
}

// providerRows projects config.providers onto display rows, sorted by name.
// fallbackBase is the global base_url, used when a remote provider omits its
// own base_url (matching provider.ResolveCmdProvider's precedence).
func providerRows(providers map[string]*types.NamedProvider, fallbackBase string) []providerRow {
	names := make([]string, 0, len(providers))
	for name, np := range providers {
		if np != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	rows := make([]providerRow, 0, len(names))
	for _, name := range names {
		np := providers[name]
		row := providerRow{name: name, typ: string(np.Type)}
		if row.typ == "" {
			row.typ = string(types.ProviderOpenAI)
		}

		if np.Type == types.ProviderLocal || np.Type == types.ProviderOllama {
			row.host, row.key = hostOf(np.BaseURL), "n/a"
			rows = append(rows, row)
			continue
		}

		base := np.BaseURL
		if base == "" {
			base = fallbackBase
		}
		if base == "" {
			base = types.DefaultAPIBaseURL
		}
		row.host = hostOf(base)
		switch {
		case provider.IsLocalEndpoint(base):
			row.key = "n/a"
		case np.APIKey != "":
			row.key = "yes"
		default:
			row.key = "no"
		}
		rows = append(rows, row)
	}
	return rows
}

// hostOf extracts only the host name from a base URL. Userinfo, port, path and
// query parameters are dropped, so credentials can never leak into the list.
func hostOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "-"
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "-"
	}
	return u.Hostname()
}

// writeProviderTable prints one row per configured provider. API key values are
// never printed — only whether one is configured.
func writeProviderTable(w io.Writer, rows []providerRow) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tTYPE\tHOST\tKEY"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", row.name, row.typ, row.host, row.key); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// runListProviders prints the configured provider registry without any HTTP
// request, so it works with no API key at all.
func runListProviders() error {
	rows := providerRows(d.Providers, d.APIBase)
	if len(rows) == 0 {
		fmt.Println("No providers configured. Add entries under `providers:` in config.yaml.")
		return nil
	}

	fmt.Printf("Configured providers (%d):\n\n", len(rows))
	if err := writeProviderTable(os.Stdout, rows); err != nil {
		return fmt.Errorf("write provider list: %w", err)
	}
	fmt.Println()
	fmt.Println("Use:    aigc-cli models --provider <name>")
	fmt.Println("No key: aigc-cli models --type image | aigc-cli models --price")
	return nil
}

// missingKey reports whether p needs an API key that is not configured.
func missingKey(p *provider.EffectiveProvider) bool {
	return p != nil && p.RequiresAPIKey() && !provider.IsLocalEndpoint(p.BaseURL)
}

// noKeyFallback lists candidate providers before returning the missing-key
// error, so the user can pick one without opening config.yaml.
func noKeyFallback(p *provider.EffectiveProvider) error {
	base := p.BaseURL
	if base == "" {
		base = types.DefaultAPIBaseURL
	}
	fmt.Printf("No API key for models (resolved base URL: %s).\n\n", base)

	rows := providerRows(d.Providers, d.APIBase)
	if len(rows) == 0 {
		fmt.Println("No providers configured. Pass --api-key, set OPENAI_API_KEY, or add providers to config.yaml.")
	} else {
		fmt.Printf("Candidate providers (%d):\n\n", len(rows))
		if err := writeProviderTable(os.Stdout, rows); err != nil {
			return fmt.Errorf("write provider list: %w", err)
		}
		fmt.Println()
		fmt.Println("Pick one with: aigc-cli models --provider <name>")
		fmt.Println("No key needed: aigc-cli models --type image | aigc-cli models --price")
	}

	return fmt.Errorf("no API key for models: use --provider <name> or pass --api-key / set OPENAI_API_KEY")
}
