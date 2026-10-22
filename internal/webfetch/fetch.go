package webfetch

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"github.com/martianzhang/aigc-cli/internal/netguard"
)

const (
	// DefaultMaxBytes bounds how much of a response body is buffered.
	DefaultMaxBytes int64 = 10 * 1024 * 1024
	// DefaultTimeout is the per-request timeout when the caller omits one.
	DefaultTimeout = 30 * time.Second
)

// Options controls one fetch. Zero values select the defaults.
type Options struct {
	MaxBytes       int64
	Timeout        time.Duration
	Headers        []Header
	UserAgent      string
	Accept         string
	AcceptLanguage string
	// ValidateURL overrides SSRF validation (tests use a no-op). Nil means
	// netguard.ValidatePublicURL.
	ValidateURL func(string) error
	// CookieJar, when set, sends matching cookies and captures Set-Cookie on
	// every response/redirect hop.
	CookieJar http.CookieJar

	// MediaDir enables saving binary responses to a file instead of rejecting
	// them. Empty keeps the text-only behaviour.
	MediaDir string
	// MediaName is the requested file name. A base name without an extension
	// gets the detected one; a name with an extension is used verbatim.
	MediaName string
	// Force overwrites an existing media file.
	Force bool
	// MediaMaxBytes bounds a media download. Zero uses DefaultMediaMaxBytes.
	MediaMaxBytes int64
	// Progress is called while a media response is streamed. total is -1 when
	// the server does not send Content-Length. It is best-effort and may be
	// called less often than every byte.
	Progress func(downloaded, total int64)
}

// Document is the structured result of one fetch.
type Document struct {
	// URL is the normalized request URL, FinalURL is the URL after redirects.
	URL, FinalURL string
	Title         string
	Content       string // Markdown for HTML, verbatim text otherwise
	ContentType   string
	RawHTML       string // populated only when the body was HTML
	Bytes         int64
	Converted     bool // true when Content came from HTML→Markdown
	Truncated     bool // response exceeded MaxBytes

	// Media fields are populated when the response was saved to disk.
	Media     bool
	MediaType string
	SavedPath string
	SHA256    string
}

// NormalizeURL trims input and adds https:// to a bare host/path.
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	if u.Scheme == "" {
		u, err = url.Parse("https://" + raw)
		if err != nil {
			return "", fmt.Errorf("invalid URL %q: %w", raw, err)
		}
	}
	return u.String(), nil
}

// Fetch downloads rawURL and converts HTML to Markdown (readability first,
// whole-document conversion as fallback). Non-HTML textual bodies are returned
// verbatim; binary bodies are rejected. Every redirect hop is SSRF-validated,
// and the client copies http.DefaultClient so the global proxy still applies.
func Fetch(ctx context.Context, rawURL string, opts Options) (*Document, error) {
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}

	validate := opts.ValidateURL
	if validate == nil {
		validate = netguard.ValidatePublicURL
	}
	if err := validate(normalized); err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	headers := EffectiveHeadersWithDefaults([]Header{
		{Name: "User-Agent", Value: firstNonEmpty(opts.UserAgent, DefaultUserAgent)},
		{Name: "Accept", Value: firstNonEmpty(opts.Accept, DefaultAccept)},
		{Name: "Accept-Language", Value: firstNonEmpty(opts.AcceptLanguage, DefaultAcceptLanguage)},
	}, opts.Headers)
	ApplyHeaders(req, headers)

	// The guarded client re-validates every redirect hop, so a public URL cannot
	// be bounced onto a loopback/private address.
	httpClient := netguard.NewGuardedClient(timeout)
	if opts.CookieJar != nil {
		httpClient.Jar = opts.CookieJar
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", normalized, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch %s: HTTP %d", normalized, resp.StatusCode)
	}

	bodyReader := bufio.NewReader(resp.Body)
	head, _ := bodyReader.Peek(512)
	contentType := resp.Header.Get("Content-Type")
	mediaType := normalizeMediaType(contentType)
	if mediaType == "" && len(head) > 0 {
		mediaType = normalizeMediaType(http.DetectContentType(head))
		contentType = mediaType
	}
	isHTML := mediaType == "text/html" || mediaType == "application/xhtml+xml" ||
		(mediaType == "" && looksLikeHTML(head))
	finalURL := normalized
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	if !isHTML && !isTextual(mediaType) {
		if opts.MediaDir == "" {
			return nil, fmt.Errorf("fetch %s: unsupported content type %q", normalized, contentType)
		}
		return saveMedia(bodyReader, opts, normalized, finalURL, mediaType, contentType, resp.ContentLength)
	}

	body, err := io.ReadAll(io.LimitReader(bodyReader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	truncated := int64(len(body)) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}

	decoded := decodeBody(body, contentType)

	doc := &Document{
		URL:         normalized,
		Title:       "",
		ContentType: contentType,
		Bytes:       int64(len(body)),
		Truncated:   truncated,
		FinalURL:    finalURL,
	}

	if isHTML {
		pageURL, perr := url.Parse(doc.FinalURL)
		if perr != nil {
			pageURL, _ = url.Parse(normalized)
		}
		title, markdown, cerr := HTMLToMarkdown(string(decoded), pageURL)
		if cerr != nil {
			return nil, fmt.Errorf("convert %s: %w", normalized, cerr)
		}
		doc.Title = title
		doc.Content = markdown
		doc.RawHTML = string(decoded)
		doc.Converted = true
		return doc, nil
	}

	doc.Content = string(decoded)
	return doc, nil
}

// decodeBody converts a body from its declared/guessed charset to UTF-8.
func decodeBody(body []byte, contentType string) []byte {
	converted, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return body
	}
	data, err := io.ReadAll(converted)
	if err != nil {
		return body
	}
	return data
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// looksLikeHTML sniffs a response body when the server omitted Content-Type.
func looksLikeHTML(body []byte) bool {
	head := strings.ToLower(strings.TrimSpace(string(body[:min(len(body), 512)])))
	return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
}

// isTextual reports whether a media type can be returned as text.
func isTextual(mediaType string) bool {
	if mediaType == "" || strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/javascript",
		"application/x-javascript", "application/x-yaml", "application/yaml":
		return true
	}
	return strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
}
