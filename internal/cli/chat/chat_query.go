package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/netguard"
	"github.com/martianzhang/aigc-cli/internal/search"
)

// fetchURLGuard rejects non-public fetch targets (SSRF protection). It is a
// package variable so tests can bypass it for loopback httptest servers.
var fetchURLGuard = netguard.ValidatePublicURL

// maxWebFetchBytes bounds how much of a response body web_fetch buffers before
// converting/paginating. HTML→markdown needs the whole document, so this is
// larger than a single page of output.
const maxWebFetchBytes = 5 * 1024 * 1024

// defaultWebFetchUA is a browser-like User-Agent; many sites return a stub or
// block requests without one.
const defaultWebFetchUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) aigc-cli/1.0"

type webFetchArgs struct {
	URL       string `json:"url"`
	MaxLength int    `json:"max_length"`
	Offset    int    `json:"offset"`
}

const (
	// defaultFetchMaxLength is how many characters a fetch returns when the
	// model omits max_length.
	defaultFetchMaxLength = 5000
	// maxFetchMaxLength caps max_length for a single fetch.
	maxFetchMaxLength = 50000
)

// executeBuiltinFetch dispatches the `fetch` primitive: a direct HTTP GET whose
// HTML is converted to markdown (main-content extraction via readability,
// falling back to whole-document conversion) so the model gets clean text and
// the context spends fewer tokens; non-HTML bodies (JSON, plain text, …) are
// returned verbatim. It is free, needs no API key, is SSRF-guarded, and
// paginates by character offset. See executeWebFetch for the third-party
// `web_fetch` primitive.
func executeBuiltinFetch(argsJSON string) string {
	var args webFetchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.URL == "" {
		return "Error: url is required"
	}
	if args.Offset < 0 {
		return "Error: offset must be non-negative"
	}
	if args.MaxLength <= 0 {
		args.MaxLength = defaultFetchMaxLength
	}
	if args.MaxLength > maxFetchMaxLength {
		args.MaxLength = maxFetchMaxLength
	}

	if err := fetchURLGuard(args.URL); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer fetchCancel()
	httpReq, err := http.NewRequestWithContext(fetchCtx, "GET", args.URL, nil)
	if err != nil {
		return fmt.Sprintf("Error: invalid URL %s: %v", args.URL, err)
	}
	httpReq.Header.Set("User-Agent", defaultWebFetchUA)
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.5")

	// The guarded client re-validates every redirect hop, so a public URL
	// cannot be bounced onto a loopback/private address.
	resp, err := netguard.NewGuardedClient(30 * time.Second).Do(httpReq)
	if err != nil {
		return fmt.Sprintf("Error: failed to fetch %s: %v", args.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Sprintf("Error: %s returned status %d", args.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxWebFetchBytes+1))
	if err != nil {
		return fmt.Sprintf("Error: failed to read response: %v", err)
	}
	truncatedFetch := len(body) > maxWebFetchBytes
	if truncatedFetch {
		body = body[:maxWebFetchBytes]
	}

	contentType := resp.Header.Get("Content-Type")
	text := string(body)
	title := ""
	converted := false
	if isHTMLBody(contentType, body) {
		if decoded, derr := decodeCharset(body, contentType); derr == nil {
			body = decoded
		}
		var pageURL *url.URL
		if u, perr := url.Parse(args.URL); perr == nil {
			pageURL = u
		}
		if t, mdText, cerr := knowledge.HTMLToMarkdown(string(body), pageURL); cerr == nil {
			title = t
			text = mdText
			converted = true
		}
	}

	out := renderFetch("fetch", args.URL, title, text, args.Offset, args.MaxLength, converted)
	if truncatedFetch {
		out += fmt.Sprintf("\n\nNote: response exceeded %d MB and was truncated before conversion.", maxWebFetchBytes/(1024*1024))
	}
	return out
}

// renderFetch renders a fetched page body for the model, paginated by character
// offset. markdown marks the body as already-Markdown; toolName is used in the
// "read the next section" hint.
func renderFetch(toolName, url, title, text string, offset, maxLength int, markdown bool) string {
	chunk, total, start, end, remaining := takeChars(text, offset, maxLength)
	if total > 0 && start >= total {
		return fmt.Sprintf("Content has %d chars, but offset %d is past the end. Use offset=0 to read from the beginning.", total, offset)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Content from %s", url)
	if markdown {
		b.WriteString(" (markdown)")
	}
	b.WriteString("\n")
	if title != "" {
		fmt.Fprintf(&b, "Title: %s\n", title)
	}
	fmt.Fprintf(&b, "Fetched: %d chars\n", total)
	fmt.Fprintf(&b, "Showing chars %d-%d", start, end)
	if remaining > 0 {
		fmt.Fprintf(&b, " (%d remaining)\n\n", remaining)
	} else {
		b.WriteString("\n\n")
	}
	b.WriteString(chunk)
	if remaining > 0 {
		fmt.Fprintf(&b, "\n\nContent truncated. Use %s(url=%q, offset=%d) to read the next section.", toolName, url, end)
	}
	return b.String()
}

// executeWebFetch dispatches the `web_fetch` primitive: it retrieves a page
// through the configured third-party fetch API (the same provider web_search
// uses) and returns provider-formatted Markdown. The provider may bill per
// request. For the built-in free fetcher use `fetch`.
func executeWebFetch(argsJSON string) string {
	var args webFetchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.URL == "" {
		return "Error: url is required"
	}
	if args.Offset < 0 {
		return "Error: offset must be non-negative"
	}
	if args.MaxLength <= 0 {
		args.MaxLength = defaultFetchMaxLength
	}
	if args.MaxLength > maxFetchMaxLength {
		args.MaxLength = maxFetchMaxLength
	}

	router := remoteRouter()
	fetcher, ok := search.ResolveFetcher(router, resolveSearchVendor())
	if !ok {
		return "Error: no third-party fetch provider configured. Configure a fetch-capable provider under web_search.*, or use the built-in `fetch` primitive."
	}
	title, content, err := fetcher.Fetch(args.URL)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return renderFetch("web_fetch", args.URL, title, content, args.Offset, args.MaxLength, true)
}

// isHTMLBody reports whether a response should be treated as HTML and converted
// to markdown. An explicit non-HTML Content-Type (JSON, plain text, XML, binary)
// wins over sniffing; only a missing/blank type falls back to looking at the
// body's opening bytes.
func isHTMLBody(contentType string, body []byte) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.Contains(ct, "html") { // text/html, application/xhtml+xml
		return true
	}
	if ct != "" {
		return false
	}
	head := strings.ToLower(strings.TrimSpace(string(body[:min(len(body), 512)])))
	return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
}

// decodeCharset converts a body from its declared/guessed charset to UTF-8.
func decodeCharset(body []byte, contentType string) ([]byte, error) {
	r, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return nil, err
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// takeChars returns the rune slice [offset, offset+maxLen) of s, clamped to the
// string, plus the total rune count, the clamped start and end, and how many
// runes remain. Counting runes (not bytes) keeps the model's offset cursor from
// landing inside a multi-byte character.
func takeChars(s string, offset, maxLen int) (chunk string, total, start, end, remaining int) {
	runes := []rune(s)
	total = len(runes)
	start = min(max(offset, 0), total)
	end = min(start+maxLen, total)
	return string(runes[start:end]), total, start, end, total - end
}

func printToolArgs(argsJSON string) {
	if argsJSON == "" || argsJSON == "{}" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		raw := argsJSON
		if len(raw) > 120 {
			raw = raw[:120] + "..."
		}
		fmt.Fprintf(options.Stderr(), "  %s\r\n", raw)
		return
	}
	for k, v := range m {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "..."
		}
		fmt.Fprintf(options.Stderr(), "  %s=%s\r\n", k, s)
	}
}
