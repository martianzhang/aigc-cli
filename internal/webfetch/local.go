package webfetch

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// LocalFilePath reports whether raw refers to an existing local file. It checks
// os.Stat first, so plain relative/absolute paths (and even a filename that
// looks like a host) win over URL interpretation. file:// URLs are also
// accepted for convenience.
//
// A missing file:// URL is an error; a non-existent plain string is simply not
// local (false) so the caller can treat it as a URL.
func LocalFilePath(raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, nil
	}

	if info, err := os.Stat(raw); err == nil {
		if info.IsDir() {
			return "", false, fmt.Errorf("%s is a directory, not a file", raw)
		}
		return absolutePath(raw)
	}

	if strings.HasPrefix(strings.ToLower(raw), "file://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", false, fmt.Errorf("invalid file URL %q: %w", raw, err)
		}
		if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
			// On Windows, URLs like file:///C:/path are parsed as
			// scheme="file" host="C:" path="/...".  Reassemble the
			// drive-letter host into the path so os.Stat can resolve it.
			if strings.ContainsRune(u.Host, ':') {
				// host includes the drive colon, e.g. "C:"
				drive := strings.ToLower(u.Host[:1])
				if len(drive) == 1 && drive >= "a" && drive <= "z" {
					u.Path = u.Host + u.Path
					u.Host = ""
				} else {
					return "", false, fmt.Errorf("file URL host %q is not supported", u.Host)
				}
			} else {
				return "", false, fmt.Errorf("file URL host %q is not supported", u.Host)
			}
		}
		path := u.Path
		// On Windows the path component may contain URL-encoded backslashes
		// (e.g. %5C) or a mix of forward/back slashes (e.g. from file:///C:/a/b).
		// url.PathUnescape restores encoded characters, and FromSlash normalises
		// any forward slashes to the platform separator so os.Stat can resolve it.
		if unescaped, err := url.PathUnescape(path); err == nil {
			path = unescaped
		}
		path = filepath.FromSlash(path)
		info, err := os.Stat(path)
		if err != nil {
			return "", false, fmt.Errorf("read local file %s: %w", path, err)
		}
		if info.IsDir() {
			return "", false, fmt.Errorf("%s is a directory, not a file", path)
		}
		return absolutePath(path)
	}

	return "", false, nil
}

func absolutePath(path string) (string, bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false, fmt.Errorf("resolve %s: %w", path, err)
	}
	return abs, true, nil
}

// FetchLocalFile reads a local HTML/text file and converts HTML to Markdown.
// It performs no network I/O.
func FetchLocalFile(path string, opts Options) (*Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read local file %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a file", path)
	}

	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open local file %s: %w", path, err)
	}
	defer f.Close()

	body, err := readLimited(f, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("read local file %s: %w", path, err)
	}
	truncated := int64(len(body)) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}

	contentType := contentTypeForPath(path, body)
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	isHTML := mediaType == "text/html" || mediaType == "application/xhtml+xml" ||
		(mediaType == "" && looksLikeHTML(body))
	if !isHTML && !isTextual(mediaType) {
		return nil, fmt.Errorf("unsupported content type %q for %s", contentType, path)
	}

	decoded := decodeBody(body, contentType)
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	fileURL := (&url.URL{Scheme: "file", Path: absolute}).String()

	doc := &Document{
		URL:         fileURL,
		FinalURL:    fileURL,
		ContentType: contentType,
		Bytes:       int64(len(body)),
		Truncated:   truncated,
	}
	if isHTML {
		pageURL, _ := url.Parse(fileURL)
		title, markdown, cerr := HTMLToMarkdown(string(decoded), pageURL)
		if cerr != nil {
			return nil, fmt.Errorf("convert %s: %w", path, cerr)
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

// contentTypeForPath prefers the file extension, then MIME sniffing. HTML
// extensions deliberately omit a charset so charset.NewReader can honor the
// document's <meta charset> (local files may be GBK, etc.).
func contentTypeForPath(path string, body []byte) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm":
		return "text/html"
	case ".xhtml", ".xht":
		return "application/xhtml+xml"
	}
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); ct != "" {
		return ct
	}
	return http.DetectContentType(body)
}

func readLimited(r *os.File, maxBytes int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxBytes+1))
}
