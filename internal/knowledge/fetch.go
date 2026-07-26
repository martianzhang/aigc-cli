package knowledge

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// defaultUserAgent is used for web requests.
const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) aigc-cli-kb/1.0"

// httpClient is the shared HTTP client for KB operations.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

// FetchResult holds the result of a URL fetch.
type FetchResult struct {
	URL         string
	Title       string
	Content     string // markdown content
	HTML        string // raw HTML
	ContentType string
	Size        int64
}

// FetchURL fetches a URL, extracts the main content via readability,
// and converts to markdown.
func FetchURL(rawURL string) (*FetchResult, error) {
	// Auto-add https:// for bare hostnames
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing host in URL %q", rawURL)
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", u.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch %s: HTTP %d", u.String(), resp.StatusCode)
	}

	// Detect and decode charset
	contentType := resp.Header.Get("Content-Type")
	body, err := charset.NewReader(resp.Body, contentType)
	if err != nil {
		body = resp.Body
	}

	htmlBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	size := int64(len(htmlBytes))
	if size > 10*1024*1024 {
		return nil, fmt.Errorf("response too large: %d MB", size/(1024*1024))
	}

	htmlStr := string(htmlBytes)

	// Extract main content via readability
	article, err := readability.FromReader(strings.NewReader(htmlStr), u)
	if err != nil {
		return nil, fmt.Errorf("readability extract: %w", err)
	}
	title := article.Title()

	// Render article HTML and convert to markdown
	var htmlBuf strings.Builder
	if err := article.RenderHTML(&htmlBuf); err != nil {
		return nil, fmt.Errorf("render article HTML: %w", err)
	}
	markdown, err := md.ConvertString(htmlBuf.String())
	if err != nil {
		return nil, fmt.Errorf("html-to-markdown: %w", err)
	}

	return &FetchResult{
		URL:         u.String(),
		Title:       title,
		Content:     markdown,
		HTML:        htmlStr,
		ContentType: contentType,
		Size:        size,
	}, nil
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

// projectDir converts a project ID (e.g., "github.com/martianzhang/aigc-cli")
// to a filesystem-safe directory name.
func projectDir(project string) string {
	s := strings.ReplaceAll(project, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}

// SaveDocFile saves raw content to baseDir/docs/<project-dir>/<id>-<title>.md.
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
	name := shortID + "-" + slug + ".md"
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

// ddgClient is a dedicated HTTP client for DDG HTML searches with a short timeout.
var ddgClient = &http.Client{Timeout: 10 * time.Second}

// DDGSearchURLs performs a DuckDuckGo HTML search and returns result URLs.
func DDGSearchURLs(query string) ([]string, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; aigc-cli/1.0)")

	resp, err := ddgClient.Do(req)
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

	if len(urls) > 3 {
		urls = urls[:3]
	}
	return urls, nil
}
