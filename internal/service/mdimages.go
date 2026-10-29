package service

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/fsutil"
	"github.com/martianzhang/aigc-cli/internal/imgcodec"
)

// imageSpan is one remote image reference inside a Markdown document:
// the byte range of the URL and the URL itself.
type imageSpan struct {
	start int
	end   int
	url   string
}

// markdownImageRe matches `![alt](url)` and `![alt](<url>)` for http(s) URLs.
// Group 1 is the angle-bracket form, group 2 the plain form.
var markdownImageRe = regexp.MustCompile(`!\[[^\]\r\n]*\]\(\s*(?:<(https?://[^>\r\n]+)>|(https?://[^\s)\r\n]+))`)

// htmlImageRe matches `src="http…"` / `src='http…'` on <img> tags. RE2 has no
// backreferences, so each quote style is its own group.
var htmlImageRe = regexp.MustCompile(`<img\b[^>]*?\bsrc\s*=\s*(?:"(https?://[^"]+)"|'(https?://[^']+)')`)

// LocalizeImagesResult reports what an image localization pass did.
type LocalizeImagesResult struct {
	// Saved holds the file names written into the asset directory.
	Saved []string
	// Failed holds "url: reason" entries; those references stay remote.
	Failed []string
}

// LocalizeMarkdownImages downloads every remote image referenced by markdown
// (both `![alt](https://…)` and `<img src="https://…">`) into assetDir and
// rewrites the references to `linkPrefix/<file>`. The asset directory is only
// created when at least one image is saved.
//
// Downloads are best effort: a failure leaves that reference pointing at the
// remote URL and is reported in Failed, because losing an image must not lose
// the parsed document. SoMark deletes parse-time image URLs after 30 days, so
// callers should surface failures rather than ignore them.
func LocalizeMarkdownImages(markdown, assetDir, linkPrefix string) (string, LocalizeImagesResult) {
	var result LocalizeImagesResult
	spans := markdownImageSpans(markdown)
	if len(spans) == 0 {
		return markdown, result
	}

	names := make(map[string]string, len(spans))
	client := &http.Client{Timeout: 2 * time.Minute}
	for _, span := range spans {
		if _, done := names[span.url]; done {
			continue
		}
		data, err := fetchImage(client, span.url)
		if err != nil {
			result.Failed = append(result.Failed, fmt.Sprintf("%s: %v", span.url, err))
			continue
		}
		name := fmt.Sprintf("image_%03d%s", len(names)+1, imageExtension(data, span.url))
		if err := fsutil.WriteFileAtomic(filepath.Join(assetDir, name), data, 0644); err != nil {
			result.Failed = append(result.Failed, fmt.Sprintf("%s: %v", span.url, err))
			continue
		}
		names[span.url] = name
		result.Saved = append(result.Saved, name)
	}
	if len(names) == 0 {
		return markdown, result
	}

	var out strings.Builder
	cursor := 0
	for _, span := range spans {
		name, ok := names[span.url]
		if !ok {
			continue
		}
		out.WriteString(markdown[cursor:span.start])
		out.WriteString((&url.URL{Path: linkPrefix + "/" + name}).String())
		cursor = span.end
	}
	out.WriteString(markdown[cursor:])
	return out.String(), result
}

// markdownImageSpans finds remote image references in Markdown order,
// supporting `![alt](url)`, `![alt](<url>)` and HTML `<img src="url">`.
// Overlapping matches are dropped so one URL range is rewritten once.
func markdownImageSpans(markdown string) []imageSpan {
	var spans []imageSpan
	for _, group := range markdownImageRe.FindAllStringSubmatchIndex(markdown, -1) {
		// Group 1 is the angle-bracket form, group 2 the plain form.
		start, end := -1, -1
		if group[2] >= 0 {
			start, end = group[2], group[3]
		} else if group[4] >= 0 {
			start, end = group[4], group[5]
		}
		if start < 0 {
			continue
		}
		spans = append(spans, imageSpan{start: start, end: end, url: markdown[start:end]})
	}
	for _, group := range htmlImageRe.FindAllStringSubmatchIndex(markdown, -1) {
		start, end := -1, -1
		if group[2] >= 0 {
			start, end = group[2], group[3]
		} else if group[4] >= 0 {
			start, end = group[4], group[5]
		}
		if start < 0 {
			continue
		}
		spans = append(spans, imageSpan{start: start, end: end, url: markdown[start:end]})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	kept := spans[:0]
	previousEnd := -1
	for _, span := range spans {
		if span.start < previousEnd {
			continue
		}
		kept = append(kept, span)
		previousEnd = span.end
	}
	return kept
}

// maxImageBytes caps one download so a misbehaving URL cannot exhaust memory.
const maxImageBytes = 64 << 20

// fetchImage downloads one image with two retries for transient failures.
func fetchImage(client *http.Client, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		data, retryable, err := downloadImage(client, rawURL)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retryable {
			break
		}
	}
	return nil, lastErr
}

// downloadImage performs one GET and reports whether the failure is transient.
func downloadImage(client *http.Client, rawURL string) ([]byte, bool, error) {
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retryable, fmt.Errorf("status %d", resp.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if contentType != "" && !strings.HasPrefix(contentType, "image/") && contentType != "application/octet-stream" {
		return nil, false, fmt.Errorf("unexpected content type %q", contentType)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, true, err
	}
	if len(data) > maxImageBytes {
		return nil, false, fmt.Errorf("image exceeds %d MB", maxImageBytes>>20)
	}
	if len(data) == 0 {
		return nil, false, fmt.Errorf("empty response")
	}
	return data, false, nil
}

// imageExtension prefers the sniffed format, then the URL suffix.
func imageExtension(data []byte, rawURL string) string {
	if ext := imgcodec.SniffImageExt(data); ext != "" {
		return ext
	}
	if ext := ExtractImageExt(rawURL); ext != "" {
		return ext
	}
	return ".png"
}
