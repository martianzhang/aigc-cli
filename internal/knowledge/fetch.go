package knowledge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/martianzhang/aigc-cli/internal/netguard"
	"github.com/martianzhang/aigc-cli/internal/webfetch"
)

// defaultUserAgent is the browser-like UA shared by the fetch core.
const defaultUserAgent = webfetch.DefaultUserAgent

// maxFetchBytes caps how much of a response body FetchURL reads into memory.
const maxFetchBytes = 10 * 1024 * 1024

// FetchResult holds the result of a URL fetch.
type FetchResult struct {
	URL         string
	Title       string
	Content     string // markdown content
	HTML        string // raw HTML
	ContentType string
	Size        int64
}

// FetchURL fetches a URL, extracts the main content via readability, and
// converts HTML to markdown. Non-HTML textual responses are returned verbatim.
func FetchURL(rawURL string) (*FetchResult, error) {
	doc, err := webfetch.Fetch(context.Background(), rawURL, webfetch.Options{
		MaxBytes:  maxFetchBytes,
		Timeout:   30 * time.Second,
		UserAgent: defaultUserAgent,
		Accept:    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
	})
	if err != nil {
		return nil, err
	}
	if doc.Truncated {
		return nil, fmt.Errorf("response too large: %d MB", doc.Bytes/(1024*1024))
	}
	return &FetchResult{
		URL:         doc.URL,
		Title:       doc.Title,
		Content:     doc.Content,
		HTML:        doc.RawHTML,
		ContentType: doc.ContentType,
		Size:        doc.Bytes,
	}, nil
}

// HTMLToMarkdown extracts the main readable content of an HTML document and
// converts it to markdown. It is kept as a knowledge-package entry point for
// existing callers and tests; the implementation lives in internal/webfetch.
func HTMLToMarkdown(htmlStr string, pageURL *url.URL) (title, markdown string, err error) {
	return webfetch.HTMLToMarkdown(htmlStr, pageURL)
}

// Checksum computes the SHA256 hex digest of content.
func Checksum(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:])
}

// DocID computes a document ID from content.
func DocID(content string) string {
	return Checksum(content)
}

// projectDir converts a project ID to a cross-platform-safe directory name.
func projectDir(project string) string {
	s := make([]byte, 0, len(project))
	for _, r := range project {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			s = append(s, byte(r))
		} else {
			s = append(s, '_')
		}
	}
	return string(s)
}

// SaveDocFile saves raw content to baseDir/docs/<project-dir>/<date>-<id>-<title>.md.
// If project is empty, saves to baseDir/docs/global/.
func SaveDocFile(baseDir, project, docID, title, content string) error {
	dirName := "global"
	if project != "" {
		dirName = projectDir(project)
	}
	dir := filepath.Join(baseDir, "docs", dirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create docs dir: %w", err)
	}
	date := time.Now().Format("2006-01-02")
	shortID := docID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	slug := slugify(title)
	if slug == "" {
		slug = "untitled"
	}
	if len(slug) > 60 {
		slug = slug[:60]
	}
	name := date + "_" + shortID + "-" + slug + ".md"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write doc file: %w", err)
	}
	return nil
}

// slugify makes a string filesystem-safe.
func slugify(s string) string {
	var out []byte
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, byte(r))
		case r >= 'A' && r <= 'Z':
			out = append(out, byte(r+'a'-'A')) // lowercase
		case r == ' ' || r == '-' || r == '_':
			out = append(out, '-')
		case r > 127:
			// CJK and other unicode: transliterate loosely
			// Just skip for now; the hash prefix ensures uniqueness
		}
	}
	// Collapse multiple dashes
	result := string(out)
	for i := 0; i < len(result); i++ {
		if result[i] == '-' {
			for i+1 < len(result) && result[i+1] == '-' {
				result = result[:i+1] + result[i+2:]
			}
		}
	}
	return result
}

// DDGSearchURLs performs a DuckDuckGo HTML search and returns result URLs.
func DDGSearchURLs(query string) ([]string, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; aigc-cli/1.0)")

	resp, err := netguard.NewGuardedClient(10 * time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	var urls []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "result__a") {
					for _, a := range n.Attr {
						if a.Key == "href" {
							u := a.Val
							if strings.Contains(u, "uddg=") {
								if parsed, err := url.Parse(u); err == nil {
									u = parsed.Query().Get("uddg")
								}
							}
							if u != "" && !strings.HasPrefix(u, "/") {
								urls = append(urls, u)
							}
						}
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Search results are attacker-influenceable: drop anything pointing at a
	// non-public address before it ever reaches FetchURL.
	safe := make([]string, 0, len(urls))
	for _, u := range urls {
		if err := netguard.ValidatePublicURL(u); err != nil {
			continue
		}
		safe = append(safe, u)
	}
	urls = safe

	if len(urls) > 3 {
		urls = urls[:3]
	}
	return urls, nil
}
