package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/webfetch"
)

// fetchDocument is a package variable so command tests can avoid network I/O.
var fetchDocument = webfetch.Fetch
var fetchLocalFile = webfetch.FetchLocalFile
var localFilePath = webfetch.LocalFilePath

// fetchFlags holds the parsed flags for one `aigc-cli fetch` invocation.
type fetchFlags struct {
	provider      string
	maxChars      int
	headers       []string
	cookies       []string
	cookieJar     string
	dryRun        bool
	html          bool
	listProviders bool
}

// FetchCmd returns the `fetch` command.
func FetchCmd() *cobra.Command {
	flags := &fetchFlags{}
	cmd := &cobra.Command{
		Use:   "fetch <url>",
		Short: "Fetch a URL or local file and print clean Markdown",
		Long: `Fetch a URL or a local HTML/text file and print its main content as Markdown
on stdout.
The argument can be an http(s) URL or any existing local file. Local files are
detected with os.Stat first (a file:// URL also works); they are converted with
the built-in HTML→Markdown pipeline and never touch the network.

For URLs, the default is the built-in free fetcher: a direct HTTP GET with
readability extraction and HTML→Markdown conversion. Non-HTML textual bodies
(JSON, plain text, …) are printed verbatim; binary bodies are rejected. Fetches
are SSRF-guarded and honor the global HTTP proxy and --timeout.

Use --provider to route through a third-party fetch API instead (DeepSeek or
Exa; see --list-providers). That may bill per request, and -H/--header,
-b/--cookie and -c/--cookie-jar are not supported because the request runs on
the vendor's side. If the selected vendor is search-only, fetch falls back to
the built-in fetcher and prints a note on stderr. An unknown provider name is
still an error. The built-in fetcher is always the default;
defaults.search.provider is never read implicitly.

Cookies follow curl's conventions: -b accepts repeated inline data
("name=value; name2=value2") or Netscape cookie files, and -c writes the merged
jar back after fetching. -H "Cookie: ..." takes precedence over -b.

Output is only the page body, so redirection works naturally:

  aigc-cli fetch https://example.com/article > article.md

Use --html to print the raw response body (or local file contents) instead of
converted Markdown. It is only available with the built-in fetcher.

--dry-run prints the equivalent curl command without making any request.`,
		Example: `  aigc-cli fetch https://go.dev/doc/effective_go
  aigc-cli fetch https://go.dev/doc/effective_go > effective_go.md
  aigc-cli fetch ./page.html > page.md
  aigc-cli fetch /tmp/report.html --max-chars 20000
  aigc-cli fetch https://example.com/private -H "Authorization: Bearer $TOKEN"
  aigc-cli fetch https://example.com/private -b cookies.txt
  aigc-cli fetch https://example.com/private -b a.txt -b b.txt -c cookies.txt
  aigc-cli fetch https://example.com/private -b "session=abc; theme=dark"
  aigc-cli fetch https://example.com/app --provider deepseek
  aigc-cli fetch https://example.com/article --max-chars 20000
  aigc-cli fetch https://example.com/article --html > page.html
  aigc-cli fetch --list-providers
  aigc-cli fetch https://example.com/article --dry-run`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.listProviders {
				return writeProviderList(cmd.OutOrStdout(), newSearchRouter())
			}
			if len(args) != 1 {
				return fmt.Errorf("url is required (or use --list-providers)")
			}
			return runFetch(cmd.OutOrStdout(), cmd.ErrOrStderr(), args[0], *flags, newSearchRouter())
		},
	}

	f := cmd.Flags()
	f.StringVarP(&flags.provider, "provider", "P", "", "Fetch provider name (see --list-providers); empty = built-in; search-only providers fall back to built-in")
	f.IntVar(&flags.maxChars, "max-chars", 0, "Truncate output to N characters (runes); 0 = no limit")
	f.StringArrayVarP(&flags.headers, "header", "H", nil, `Extra header "Name: value"; repeatable; "Name:" removes a default header (built-in fetcher only)`)
	f.StringArrayVarP(&flags.cookies, "cookie", "b", nil, `Cookie data "name=value; ..." or a Netscape cookie file; repeatable (built-in fetcher only)`)
	f.StringVarP(&flags.cookieJar, "cookie-jar", "c", "", "Write cookies to a Netscape-format file after fetching (built-in fetcher only)")
	f.BoolVar(&flags.html, "html", false, "Print the raw HTML/response body instead of converted Markdown (built-in fetcher only)")
	f.BoolVar(&flags.dryRun, "dry-run", false, "Print the equivalent curl command without making any request")
	f.BoolVar(&flags.listProviders, "list-providers", false, "List configured providers with search/fetch capability and availability")

	registerProviderCompletion(cmd, true)
	return cmd
}

// runFetch dispatches to the built-in or third-party fetcher and writes the
// body to stdout. Diagnostics go to stderr only with -v.
func runFetch(stdout, stderr io.Writer, rawURL string, flags fetchFlags, router *search.Router) error {
	headers, err := webfetch.ParseHeaders(flags.headers)
	if err != nil {
		return err
	}
	headers, cookieHeader, cookieHeaderFound := extractCookieHeader(headers)

	if path, isLocal, err := localFilePath(rawURL); err != nil {
		return err
	} else if isLocal {
		if flags.provider != "" || len(headers) > 0 || cookieHeaderFound || len(flags.cookies) > 0 || flags.cookieJar != "" {
			return fmt.Errorf("local files use the built-in HTML→Markdown converter; --provider/--header/--cookie/--cookie-jar are not applicable")
		}
		return runLocalFetch(stdout, stderr, path, flags)
	}

	if flags.provider == "" {
		return runBuiltinFetch(stdout, stderr, rawURL, headers, cookieHeader, cookieHeaderFound, flags)
	}

	fetcher, ok, err := resolveFetcher(router, flags.provider)
	if err != nil {
		return err
	}
	if !ok {
		warnFetchFallback(stderr, flags.provider, router)
		return runBuiltinFetch(stdout, stderr, rawURL, headers, cookieHeader, cookieHeaderFound, flags)
	}
	if flags.html {
		return fmt.Errorf("--html is only supported by the built-in fetcher; third-party fetch returns processed content")
	}
	if len(headers) > 0 || cookieHeaderFound || len(flags.cookies) > 0 || flags.cookieJar != "" {
		return fmt.Errorf("--header, --cookie and --cookie-jar are only supported by the built-in fetcher; third-party fetch runs on the vendor's side")
	}
	return runProviderFetch(stdout, stderr, rawURL, flags, fetcher)
}

// extractCookieHeader pulls the explicit Cookie header out of the header list so
// it can be applied through the cookie store (and take precedence over -b).
func extractCookieHeader(headers []webfetch.Header) ([]webfetch.Header, string, bool) {
	rest := make([]webfetch.Header, 0, len(headers))
	var values []string
	found := false
	for _, h := range headers {
		if strings.EqualFold(h.Name, "Cookie") {
			found = true
			if strings.TrimSpace(h.Value) != "" {
				values = append(values, h.Value)
			}
			continue
		}
		rest = append(rest, h)
	}
	return rest, strings.Join(values, "; "), found
}

func runLocalFetch(stdout, stderr io.Writer, path string, flags fetchFlags) error {
	if flags.dryRun {
		fmt.Fprintln(stdout, "# dry run: no request sent")
		fmt.Fprintf(stdout, "# source: local file %s\n", path)
		return nil
	}

	doc, err := fetchLocalFile(path, webfetch.Options{})
	if err != nil {
		return err
	}
	content := fetchedBody(doc, flags.html)
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("no readable text found in %s", path)
	}
	if options.Shared.Verbose {
		fmt.Fprintf(stderr, "Source: local file\nPath: %s\nTitle: %s\nContent-Type: %s\nBytes: %d\nConverted: %v\n",
			path, doc.Title, doc.ContentType, doc.Bytes, doc.Converted)
	}
	return writeFetchedContent(stdout, stderr, content, flags.maxChars)
}

// fetchedBody selects raw HTML when requested, falling back to the text content
// for non-HTML responses.
func fetchedBody(doc *webfetch.Document, rawHTML bool) string {
	if rawHTML && doc.RawHTML != "" {
		return doc.RawHTML
	}
	return doc.Content
}

func runBuiltinFetch(stdout, stderr io.Writer, rawURL string, headers []webfetch.Header, cookieHeader string, cookieHeaderFound bool, flags fetchFlags) error {
	normalized, err := webfetch.NormalizeURL(rawURL)
	if err != nil {
		return err
	}

	store, err := buildCookieStore(normalized, flags.cookies, cookieHeader, cookieHeaderFound)
	if err != nil {
		return err
	}

	if flags.dryRun {
		plan, err := builtinFetchPlan(normalized, headers)
		if err != nil {
			return err
		}
		return writeCurlPreview(stdout, "built-in", plan, cookieCurlArgs(flags, cookieHeader, cookieHeaderFound))
	}

	doc, fetchErr := fetchDocument(context.Background(), normalized, webfetch.Options{
		Timeout:   fetchTimeout(),
		Headers:   headers,
		CookieJar: store,
	})
	if flags.cookieJar != "" {
		if werr := store.WriteNetscapeFile(flags.cookieJar); werr != nil && fetchErr == nil {
			return fmt.Errorf("write cookie jar: %w", werr)
		}
	}
	if fetchErr != nil {
		return fetchErr
	}
	content := fetchedBody(doc, flags.html)
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("no readable text found at %s (the page may be a JavaScript redirect or an anti-bot stub); try --provider deepseek", doc.FinalURL)
	}
	if options.Shared.Verbose {
		fmt.Fprintf(stderr, "Provider: built-in\nTitle: %s\nFinal URL: %s\nContent-Type: %s\nBytes: %d\nConverted: %v\n",
			doc.Title, doc.FinalURL, doc.ContentType, doc.Bytes, doc.Converted)
	}
	return writeFetchedContent(stdout, stderr, content, flags.maxChars)
}

// buildCookieStore merges -b sources and an explicit Cookie header. The header
// is applied last so it wins over same-name cookies from -b.
func buildCookieStore(normalizedURL string, args []string, cookieHeader string, cookieHeaderFound bool) (*webfetch.CookieStore, error) {
	u, err := url.Parse(normalizedURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL for cookies: %w", err)
	}

	store := webfetch.NewCookieStore()
	for _, arg := range args {
		if strings.Contains(arg, "=") {
			cookies, err := webfetch.ParseCookieString(arg)
			if err != nil {
				return nil, err
			}
			store.SetCookies(u, cookies)
			continue
		}
		cookies, err := webfetch.ParseNetscapeFile(arg)
		if err != nil {
			return nil, err
		}
		for _, c := range cookies {
			store.AddCookie(c)
		}
	}

	if cookieHeaderFound {
		if strings.TrimSpace(cookieHeader) == "" {
			store.Clear()
		} else {
			cookies, err := webfetch.ParseCookieString(cookieHeader)
			if err != nil {
				return nil, fmt.Errorf("parse -H Cookie: %w", err)
			}
			store.SetCookies(u, cookies)
		}
	}
	return store, nil
}

func runProviderFetch(stdout, stderr io.Writer, rawURL string, flags fetchFlags, fetcher search.Fetcher) error {
	normalized, err := webfetch.NormalizeURL(rawURL)
	if err != nil {
		return err
	}

	if flags.dryRun {
		previewer, ok := fetcher.(search.FetchPreviewer)
		if !ok {
			return fmt.Errorf("provider %q does not provide a curl preview", fetcher.Name())
		}
		plan, err := previewer.PreviewFetch(normalized)
		if err != nil {
			return err
		}
		return writeCurlPreview(stdout, fetcher.Name(), plan, nil)
	}

	title, content, err := fetcher.Fetch(normalized)
	if err != nil {
		return fmt.Errorf("fetch via %s: %w", fetcher.Name(), err)
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("provider %q returned empty content for %s", fetcher.Name(), normalized)
	}
	if options.Shared.Verbose {
		fmt.Fprintf(stderr, "Provider: %s\nTitle: %s\nChars: %d\n", fetcher.Name(), title, len([]rune(content)))
	}
	return writeFetchedContent(stdout, stderr, content, flags.maxChars)
}

// resolveFetcher returns the named fetch-capable provider. The boolean is
// false when the name is a configured search-only provider (callers fall back
// to the built-in fetcher). Unknown names are errors.
func resolveFetcher(router *search.Router, spec string) (search.Fetcher, bool, error) {
	if fetcher, ok := search.ResolveFetcher(router, spec); ok {
		return fetcher, true, nil
	}
	if bad, ok := router.Unavailable(spec); ok {
		return nil, false, fmt.Errorf("fetch provider %q is configured but unavailable: %w", spec, bad)
	}
	if _, exists := router.GetProvider(spec); exists {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("fetch provider %q is not configured (available: %s)", spec, strings.Join(router.Names(), ", "))
}

// warnFetchFallback explains why a requested provider fell back to the
// built-in fetcher, without touching stdout.
func warnFetchFallback(stderr io.Writer, spec string, router *search.Router) {
	if provider, exists := router.GetProvider(spec); exists {
		capable := strings.Join(router.FetcherNames(), ", ")
		if capable == "" {
			capable = "none configured"
		}
		fmt.Fprintf(stderr, "Note: provider %q does not support fetch; using the built-in fetcher (fetch-capable: %s).\n",
			provider.Name(), capable)
		return
	}
	fmt.Fprintf(stderr, "Note: no fetch-capable provider matched %q; using the built-in fetcher.\n", spec)
}

// builtinFetchPlan renders the built-in request for --dry-run.
func builtinFetchPlan(normalizedURL string, headers []webfetch.Header) (*reqbuild.Plan, error) {
	effective := webfetch.EffectiveHeaders(headers)
	planHeaders := make([]reqbuild.Header, 0, len(effective))
	for _, h := range effective {
		planHeaders = append(planHeaders, reqbuild.Header{Name: h.Name, Value: h.Value})
	}
	return &reqbuild.Plan{
		Method:  http.MethodGet,
		URL:     normalizedURL,
		Headers: planHeaders,
	}, nil
}

func writeCurlPreview(w io.Writer, provider string, plan *reqbuild.Plan, extraArgs []string) error {
	fmt.Fprintln(w, "# dry run: no request sent")
	fmt.Fprintf(w, "# provider: %s\n", provider)
	_, err := fmt.Fprintln(w, appendCurlArgs(plan.RenderCurls(""), extraArgs))
	return err
}

// appendCurlArgs adds cookie flags to a rendered curl command.
func appendCurlArgs(curl string, args []string) string {
	if len(args) == 0 {
		return curl
	}
	var b strings.Builder
	b.WriteString(curl)
	for _, arg := range args {
		b.WriteString(" \\\n  ")
		b.WriteString(arg)
	}
	return b.String()
}

// cookieCurlArgs renders -b/-c flags for the preview. Inline cookie values are
// masked so a dry run cannot leak session tokens.
func cookieCurlArgs(flags fetchFlags, cookieHeader string, cookieHeaderFound bool) []string {
	var args []string
	for _, value := range flags.cookies {
		if strings.Contains(value, "=") {
			args = append(args, fmt.Sprintf("-b %q", maskCookieData(value)))
			continue
		}
		args = append(args, fmt.Sprintf("-b %q", value))
	}
	if cookieHeaderFound && strings.TrimSpace(cookieHeader) != "" {
		args = append(args, fmt.Sprintf("-b %q", maskCookieData(cookieHeader)))
	}
	if flags.cookieJar != "" {
		args = append(args, fmt.Sprintf("-c %q", flags.cookieJar))
	}
	return args
}

func maskCookieData(value string) string {
	cookies, err := webfetch.ParseCookieString(value)
	if err != nil {
		return "***"
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		parts = append(parts, c.Name+"=***")
	}
	return strings.Join(parts, "; ")
}

// writeFetchedContent writes only the body, adding a trailing newline for
// terminal usability. Truncation metadata goes to stderr when -v is set.
func writeFetchedContent(stdout, stderr io.Writer, content string, maxChars int) error {
	content, truncated := truncateContent(content, maxChars)
	if truncated && options.Shared.Verbose {
		fmt.Fprintf(stderr, "Truncated to %d characters\n", maxChars)
	}
	if _, err := io.WriteString(stdout, content); err != nil {
		return err
	}
	if !strings.HasSuffix(content, "\n") {
		if _, err := io.WriteString(stdout, "\n"); err != nil {
			return err
		}
	}
	return nil
}

// truncateContent cuts by rune count so multi-byte characters stay intact.
func truncateContent(content string, maxChars int) (string, bool) {
	if maxChars <= 0 {
		return content, false
	}
	runes := []rune(content)
	if len(runes) <= maxChars {
		return content, false
	}
	return string(runes[:maxChars]), true
}

// fetchTimeout maps the global --timeout (seconds) to the built-in fetcher,
// defaulting to the shared 30s when unset.
func fetchTimeout() time.Duration {
	if options.Shared.TimeoutFlag > 0 {
		return time.Duration(options.Shared.TimeoutFlag) * time.Second
	}
	return webfetch.DefaultTimeout
}
