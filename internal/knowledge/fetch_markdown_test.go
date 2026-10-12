package knowledge

import (
	"net/url"
	"strings"
	"testing"
)

func TestHTMLToMarkdown_extractsArticle(t *testing.T) {
	const page = `<!doctype html>
<html><head><title>Go Concurrency</title></head>
<body>
<nav><a href="/">HOME_MARKER</a></nav>
<article>
<h1>Go Concurrency</h1>
<p>Goroutines are lightweight threads managed by the Go runtime and are cheap to create.</p>
<p>Channels provide typed communication between goroutines, so use them to synchronize safely.</p>
<ul><li>goroutine</li><li>channel</li></ul>
</article>
<footer>FOOTER_MARKER</footer>
</body></html>`

	u, _ := url.Parse("https://example.com/blog/go")
	_, md, err := HTMLToMarkdown(page, u)
	if err != nil {
		t.Fatalf("HTMLToMarkdown() error = %v", err)
	}
	if !strings.Contains(md, "lightweight threads") {
		t.Errorf("markdown missing body text: %q", md)
	}
	if !strings.Contains(md, "goroutine") {
		t.Errorf("markdown missing list items: %q", md)
	}
	if strings.Contains(md, "HOME_MARKER") || strings.Contains(md, "FOOTER_MARKER") {
		t.Errorf("boilerplate not stripped: %q", md)
	}
}

func TestHTMLToMarkdown_fallbackForFragment(t *testing.T) {
	// A bare fragment has no article readability can segment; the whole-document
	// fallback must still return content.
	_, md, err := HTMLToMarkdown(`<p>hello <b>world</b></p>`, nil)
	if err != nil {
		t.Fatalf("HTMLToMarkdown() error = %v", err)
	}
	if !strings.Contains(md, "hello") || !strings.Contains(md, "world") {
		t.Errorf("fallback markdown = %q, want both words", md)
	}
}
