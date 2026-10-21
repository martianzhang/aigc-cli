// Package web implements the stateless web commands. `search` calls one
// configured search vendor and prints its raw JSON response; it never touches
// the knowledge base.
package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// stdinIsPipe reports whether stdin is a pipe/file rather than a terminal, so
// an empty `search` invocation can read the query from it without blocking.
var stdinIsPipe = func() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice == 0
}

// readStdin reads the query text from stdin; overridden in tests.
var readStdin = func() ([]byte, error) { return io.ReadAll(os.Stdin) }

const (
	// defaultSearchLimit is used when --limit is omitted or non-positive.
	defaultSearchLimit = 5
	// maxSearchLimit caps a single CLI search. It is higher than the chat/MCP
	// cap (10) because CLI results are read by humans, not fed to a model.
	maxSearchLimit = 20
)

// searchFlags holds the parsed flags for one `aigc-cli search` invocation.
type searchFlags struct {
	provider   string
	limit      int
	jsonInput  string
	injectJSON string
	dryRun     bool
}

// searchInput is the command-level --json document. Vendor-specific fields
// belong in --inject-json, not here.
type searchInput struct {
	Query    string `json:"query"`
	Provider string `json:"provider"`
	Limit    *int   `json:"limit"`
}

// searchRequest is the fully resolved request passed to runSearch.
type searchRequest struct {
	query    string
	provider string
	limit    int
	inject   json.RawMessage
	dryRun   bool
}

// SearchCmd returns the `search` command.
func SearchCmd() *cobra.Command {
	flags := &searchFlags{}
	cmd := &cobra.Command{
		Use:   "search [query...]",
		Short: "Search the web and print the provider's raw JSON response",
		Long: `Search the web through one configured provider and print that provider's
raw JSON response.

The command deliberately does not normalize results: vendors return richer
fields (snippets, scores, metadata) than a title/URL list, and keeping the raw
response makes debugging and downstream parsing straightforward.

Uses the configured search providers, with a built-in free DuckDuckGo fallback,
so it works without any configuration or API key. Results are never written to
the knowledge base; use 'aigc-cli kb search' for that.

--json supplies the command-level request (query, provider, limit); CLI flags
win over JSON fields. Both --json and --inject-json accept an inline JSON
string or a file path — stdin is reserved for the query itself, so
'echo "golang context" | aigc-cli search' works without arguments.
--inject-json adds vendor-native fields on top of the provider request: POST
providers merge it into the JSON body, GET providers add flat query parameters.
Array/object values follow the usual additive merge rules for POST; GET only
accepts scalars.

--dry-run prints the equivalent curl command for each candidate provider
without calling any search API (API keys are masked, injected fields included).

The --provider flag selects a search vendor (duckduckgo, firecrawl, brave,
doubao, deepseek, exa) or a strategy (auto, free, quality). It is unrelated to
the LLM provider used by generation commands. When omitted, the shared
defaults.search.provider config value is used, then auto.

All requests honor the global http_proxy config and HTTP_PROXY/HTTPS_PROXY/NO_PROXY
environment variables.`,
		Example: `  aigc-cli search "golang context timeout"
  aigc-cli search golang context timeout
  echo "golang context timeout" | aigc-cli search
  aigc-cli search - < query.txt
  aigc-cli search "vector database" --limit 10
  aigc-cli search "vector database" --provider deepseek
  aigc-cli search --json query.json
  aigc-cli search --json '{"query":"golang context","provider":"deepseek","limit":5}'
  aigc-cli search "golang" --inject-json '{"type":"neural"}'
  aigc-cli search "vector database" --dry-run`,
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		// The help footer's generic JSON hint mentions stdin; for search,
		// stdin carries the query instead.
		Annotations: map[string]string{"json_stdin_is_query": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := buildSearchRequest(cmd, args, flags)
			if err != nil {
				return err
			}
			return runSearch(cmd.OutOrStdout(), req, newSearchRouter())
		},
	}

	f := cmd.Flags()
	f.StringVarP(&flags.provider, "provider", "P", "", "Search provider: duckduckgo, firecrawl, brave, doubao, deepseek, exa, or auto/free/quality (not an LLM provider)")
	f.IntVarP(&flags.limit, "limit", "n", defaultSearchLimit, "Max results to request (1-20)")
	f.StringVar(&flags.jsonInput, "json", "", "Search request JSON: file path or inline JSON (fields: query, provider, limit); stdin is the query")
	f.StringVar(&flags.injectJSON, "inject-json", "", "JSON object (file path or inline) merged into the provider request (POST body or GET query params; scalars overwrite, arrays append)")
	f.BoolVar(&flags.dryRun, "dry-run", false, "Print the equivalent curl command for each candidate provider without calling any search API")

	return cmd
}

// buildSearchRequest resolves positional args, --json, and CLI flags using the
// repository-wide precedence: CLI flags > --json > config > built-in defaults.
func buildSearchRequest(cmd *cobra.Command, args []string, flags *searchFlags) (searchRequest, error) {
	if flags.jsonInput == "-" {
		return searchRequest{}, fmt.Errorf("--json does not read stdin; pass a file path or inline JSON (stdin is the query)")
	}

	var input searchInput
	if flags.jsonInput != "" {
		data, err := service.ReadJSONInput(flags.jsonInput)
		if err != nil {
			return searchRequest{}, fmt.Errorf("read --json: %w", err)
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&input); err != nil {
			return searchRequest{}, fmt.Errorf("parse --json: %w (vendor fields belong in --inject-json)", err)
		}
	}

	inject, err := service.ReadInjectJSON(flags.injectJSON)
	if err != nil {
		return searchRequest{}, fmt.Errorf("read --inject-json: %w", err)
	}

	// stdin is the query, not a JSON channel: `search -` forces it, and an
	// empty invocation reads a pipe/file automatically.
	forceStdin := len(args) == 1 && args[0] == "-"
	var query string
	if forceStdin {
		query, err = queryFromStdin()
	} else {
		query = strings.TrimSpace(strings.Join(args, " "))
		if query == "" {
			query = strings.TrimSpace(input.Query)
		}
		if query == "" && stdinIsPipe() {
			query, err = queryFromStdin()
		}
	}
	if err != nil {
		return searchRequest{}, err
	}
	if query == "" {
		if forceStdin {
			return searchRequest{}, fmt.Errorf("stdin is empty; pipe the query text into 'aigc-cli search -'")
		}
		return searchRequest{}, fmt.Errorf(`query is required (pass positional args, pipe text to stdin, or use --json '{"query":"..."}')`)
	}

	provider := flags.provider
	if provider == "" {
		provider = input.Provider
	}

	limit := defaultSearchLimit
	if input.Limit != nil {
		limit = *input.Limit
	}
	if cmd.Flags().Changed("limit") {
		limit = flags.limit
	}

	return searchRequest{
		query:    query,
		provider: provider,
		limit:    normalizeLimit(limit),
		inject:   inject,
		dryRun:   flags.dryRun,
	}, nil
}

// queryFromStdin reads the whole stdin stream and flattens it into one query
// line, so a multi-line file or heredoc works.
func queryFromStdin() (string, error) {
	data, err := readStdin()
	if err != nil {
		return "", fmt.Errorf("read query from stdin: %w", err)
	}
	return strings.Join(strings.Fields(string(data)), " "), nil
}

// newSearchRouter registers every configured provider plus the free
// DuckDuckGo fallback. NoopQuotaStore keeps the command independent of the
// knowledge base database, matching the chat/MCP search tools.
func newSearchRouter() *search.Router {
	router := search.NewRouter(search.NoopQuotaStore{})
	search.RegisterAll(router, options.WebSearchConfig())
	return router
}

// runSearch resolves the vendor, runs one search, and prints the raw response.
func runSearch(stdout io.Writer, req searchRequest, router *search.Router) error {
	spec := resolveProvider(req.provider)
	strategy, preferred := search.ResolveStrategy(spec)

	if strategy == "manual" {
		if err := requireProvider(router, preferred[0]); err != nil {
			return err
		}
	}

	limit := normalizeLimit(req.limit)
	if req.dryRun {
		return writeDryRun(stdout, req.query, strategy, preferred, limit, req.inject, router)
	}

	result, err := router.SearchWithOptions(req.query, limit, strategy, preferred, search.Options{Extra: req.inject})
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	return writeRawJSON(stdout, result)
}

// requireProvider turns an unknown --provider value into an actionable error
// listing the providers that are actually registered.
func requireProvider(router *search.Router, name string) error {
	if _, ok := router.GetProvider(name); ok {
		return nil
	}
	available := router.Names()
	if len(available) == 0 {
		return fmt.Errorf("search provider %q is not configured", name)
	}
	return fmt.Errorf("search provider %q is not configured (available: %s)",
		name, strings.Join(available, ", "))
}

// normalizeLimit applies the CLI default and cap.
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}
