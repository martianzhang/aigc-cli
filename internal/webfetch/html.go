package webfetch

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	readability "codeberg.org/readeck/go-readability/v2"
	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"golang.org/x/net/html"
)

// readabilityShortText is the threshold below which we also try a full-page
// conversion. Long articles always keep the readability result.
const readabilityShortText = 1500

// HTMLToMarkdown extracts the main readable content of an HTML document and
// converts it to markdown, returning the page title (may be empty) alongside.
// pageURL is used to resolve relative links and may be nil.
//
// It first tries readability (main-article extraction, which drops nav/boilerplate
// and saves tokens). When readability cannot segment an article — short pages,
// documentation, single-element pages — it falls back to converting the whole
// document so callers never lose content.
//
// For link-heavy pages such as homepages, readability sometimes mistakes the
// footer for the article. When the readable result is very short relative to
// the full page, the full page wins (with obvious footer/script noise removed)
// so navigation and other useful text are not lost.
func HTMLToMarkdown(htmlStr string, pageURL *url.URL) (title, markdown string, err error) {
	var readable string
	if article, aerr := readability.FromReader(strings.NewReader(htmlStr), pageURL); aerr == nil {
		title = article.Title()
		var htmlBuf strings.Builder
		if article.RenderHTML(&htmlBuf) == nil {
			if converted, cerr := md.ConvertString(htmlBuf.String()); cerr == nil && strings.TrimSpace(converted) != "" {
				readable = converted
			}
		}
	}

	if readable != "" {
		if textLen(readable) < readabilityShortText {
			if full, ferr := convertFullPage(htmlStr); ferr == nil && shouldUseFullDocument(readable, full) {
				return title, full, nil
			}
		}
		return title, sanitizeMarkdown(readable), nil
	}

	converted, cerr := convertFullPage(htmlStr)
	if cerr != nil {
		return title, "", fmt.Errorf("html-to-markdown: %w", cerr)
	}
	return title, converted, nil
}

var (
	dataImageMarkdownRE  = regexp.MustCompile(`(?i)!\[([^\]]*)\]\(\s*<?data:[^)\s]*\)`)
	dataLinkMarkdownRE   = regexp.MustCompile(`(?i)\[([^\]]*)\]\(\s*<?data:[^)\s]*\)`)
	jsLinkMarkdownRE     = regexp.MustCompile(`(?i)\[([^\]]*)\]\(\s*<?javascript:[^)\s]*\)`)
	emptyImageMarkdownRE = regexp.MustCompile(`!\[\s*\]\([^)]*\)`)
	emptyMarkdownLinkRE  = regexp.MustCompile(`(^|[^!])\[ *\]\(([^)\s]+)\)`)
	linkTextRE           = regexp.MustCompile(`(^|[^!])\[([^\]]*)\]\(([^)]*)\)`)
	dataAutoLinkRE       = regexp.MustCompile(`(?i)<data:[^>]*>`)
)

// sanitizeMarkdown drops inline data: URIs (common in base64 icons) while
// keeping their alt text and surrounding links. They are huge and useless to an
// LLM, and can dominate the output on homepage-style pages. javascript: links
// are reduced to their text for the same reason. Empty-alt images (common UI
// icons) are removed so they do not leave broken Markdown behind.
func sanitizeMarkdown(markdown string) string {
	markdown = dataImageMarkdownRE.ReplaceAllString(markdown, "$1")
	markdown = dataLinkMarkdownRE.ReplaceAllString(markdown, "$1")
	markdown = jsLinkMarkdownRE.ReplaceAllString(markdown, "$1")
	markdown = emptyImageMarkdownRE.ReplaceAllString(markdown, "")
	markdown = emptyMarkdownLinkRE.ReplaceAllString(markdown, "$1$2")
	markdown = normalizeLinkText(markdown)
	markdown = dataAutoLinkRE.ReplaceAllString(markdown, "")
	return markdown
}

// normalizeLinkText collapses whitespace/line-continuations left inside a link
// label after an inline image is removed, e.g. "[  \\\n翻译](url)" -> "[翻译](url)".
func normalizeLinkText(markdown string) string {
	return linkTextRE.ReplaceAllStringFunc(markdown, func(match string) string {
		sub := linkTextRE.FindStringSubmatch(match)
		prefix, text, target := sub[1], sub[2], sub[3]
		if !strings.ContainsAny(text, "\n\\") {
			return match
		}
		text = strings.ReplaceAll(text, "\\", "")
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			return prefix + target
		}
		return prefix + "[" + text + "](" + target + ")"
	})
}

// convertFullPage strips obvious chrome (scripts/styles/footers) and converts
// the whole document. It is used when readability produces too little text.
func convertFullPage(htmlStr string) (string, error) {
	converted, err := md.ConvertString(stripChromeHTML(htmlStr))
	if err != nil {
		return "", err
	}
	return sanitizeMarkdown(converted), nil
}

// shouldUseFullDocument decides whether the full-page markdown carries
// materially more text than a very short readability result.
func shouldUseFullDocument(readable, full string) bool {
	r := textLen(readable)
	f := textLen(full)
	if r == 0 {
		return f > 0
	}
	if f == 0 {
		return false
	}
	return r < readabilityShortText && float64(r)/float64(f) < 0.15
}

func textLen(s string) int {
	return len([]rune(strings.TrimSpace(s)))
}

// stripChromeHTML removes script/style elements and obvious footer nodes before
// full-page conversion. It intentionally keeps nav/header content, which is the
// useful part of a homepage.
func stripChromeHTML(htmlStr string) string {
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return htmlStr
	}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if shouldStripNode(child) {
				n.RemoveChild(child)
			} else {
				walk(child)
			}
			child = next
		}
	}
	walk(doc)

	var b strings.Builder
	if err := html.Render(&b, doc); err != nil {
		return htmlStr
	}
	return b.String()
}

func shouldStripNode(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.Data {
	case "script", "style", "footer":
		return true
	}
	for _, attr := range n.Attr {
		switch attr.Key {
		case "id", "class", "role":
			v := strings.ToLower(attr.Val)
			if v == "contentinfo" ||
				strings.Contains(v, "footer") ||
				strings.Contains(v, "ftcon") ||
				strings.Contains(v, "copyright") {
				return true
			}
		}
	}
	return false
}
