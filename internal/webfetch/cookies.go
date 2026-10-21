package webfetch

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// CookieStore is a small, exportable http.CookieJar. net/http/cookiejar hides
// individual cookies, but `fetch -c` needs to write every cookie (including
// host-only and HttpOnly ones) back in Netscape format.
type CookieStore struct {
	mu      sync.Mutex
	cookies map[string]cookieEntry
}

type cookieEntry struct {
	domain   string // lowercase, no leading dot
	hostOnly bool
	path     string
	secure   bool
	httpOnly bool
	expires  time.Time // zero = session cookie
	name     string
	value    string
}

func (c cookieEntry) key() string {
	return c.domain + "\x00" + c.path + "\x00" + c.name
}

// NewCookieStore returns an empty store.
func NewCookieStore() *CookieStore {
	return &CookieStore{cookies: make(map[string]cookieEntry)}
}

// SetCookies implements http.CookieJar. Empty Domain defaults to the request
// host (host-only); a leading dot marks a domain cookie.
func (s *CookieStore) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if u == nil {
		return
	}
	host := strings.ToLower(u.Hostname())
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range cookies {
		if c == nil || c.Name == "" {
			continue
		}
		entry := cookieEntry{name: c.Name, value: c.Value, path: c.Path, secure: c.Secure, httpOnly: c.HttpOnly}
		if entry.path == "" {
			entry.path = "/"
		}

		if c.Domain == "" {
			entry.domain = host
			entry.hostOnly = true
		} else {
			entry.domain = strings.TrimPrefix(strings.ToLower(c.Domain), ".")
			entry.hostOnly = !strings.HasPrefix(c.Domain, ".")
			if !domainMatches(host, entry.domain) {
				continue
			}
		}

		switch {
		case c.MaxAge < 0:
			delete(s.cookies, entry.key())
			continue
		case c.MaxAge > 0:
			entry.expires = now.Add(time.Duration(c.MaxAge) * time.Second)
		case !c.Expires.IsZero():
			entry.expires = c.Expires
		}
		if !entry.expires.IsZero() && entry.expires.Before(now) {
			delete(s.cookies, entry.key())
			continue
		}
		s.cookies[entry.key()] = entry
	}
}

// AddCookie adds one cookie, usually parsed from a Netscape file. A leading dot
// on Domain marks a domain cookie; otherwise it is host-only.
func (s *CookieStore) AddCookie(c *http.Cookie) {
	if c == nil || c.Name == "" || c.Domain == "" {
		return
	}
	now := time.Now()
	entry := cookieEntry{
		domain:   strings.TrimPrefix(strings.ToLower(c.Domain), "."),
		hostOnly: !strings.HasPrefix(c.Domain, "."),
		path:     c.Path,
		secure:   c.Secure,
		httpOnly: c.HttpOnly,
		expires:  c.Expires,
		name:     c.Name,
		value:    c.Value,
	}
	if entry.path == "" {
		entry.path = "/"
	}
	if !entry.expires.IsZero() && entry.expires.Before(now) {
		return
	}
	s.mu.Lock()
	s.cookies[entry.key()] = entry
	s.mu.Unlock()
}

// Cookies implements http.CookieJar, returning matching cookies in RFC 6265
// order (longer paths first).
func (s *CookieStore) Cookies(u *url.URL) []*http.Cookie {
	if u == nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	path := u.Path
	if path == "" {
		path = "/"
	}
	secure := u.Scheme == "https"
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	matched := make([]cookieEntry, 0, len(s.cookies))
	for _, c := range s.cookies {
		if !c.expires.IsZero() && c.expires.Before(now) {
			continue
		}
		if c.secure && !secure {
			continue
		}
		if !cookieDomainMatches(host, c) || !pathMatches(path, c.path) {
			continue
		}
		matched = append(matched, c)
	}
	sort.Slice(matched, func(i, j int) bool {
		if len(matched[i].path) != len(matched[j].path) {
			return len(matched[i].path) > len(matched[j].path)
		}
		return matched[i].name < matched[j].name
	})

	out := make([]*http.Cookie, 0, len(matched))
	for _, c := range matched {
		out = append(out, &http.Cookie{
			Name:     c.name,
			Value:    c.value,
			Path:     c.path,
			Domain:   c.domain,
			Secure:   c.secure,
			HttpOnly: c.httpOnly,
			Expires:  c.expires,
		})
	}
	return out
}

// All returns every non-expired cookie for export. Domain cookies carry a
// leading dot, matching Netscape/curl conventions.
func (s *CookieStore) All() []*http.Cookie {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]*http.Cookie, 0, len(s.cookies))
	for _, c := range s.cookies {
		if !c.expires.IsZero() && c.expires.Before(now) {
			continue
		}
		domain := c.domain
		if !c.hostOnly {
			domain = "." + domain
		}
		out = append(out, &http.Cookie{
			Name:     c.name,
			Value:    c.value,
			Path:     c.path,
			Domain:   domain,
			Secure:   c.secure,
			HttpOnly: c.httpOnly,
			Expires:  c.expires,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Clear removes every cookie (used by `-H "Cookie:"`).
func (s *CookieStore) Clear() {
	s.mu.Lock()
	s.cookies = make(map[string]cookieEntry)
	s.mu.Unlock()
}

func domainMatches(host, domain string) bool {
	host = strings.ToLower(host)
	domain = strings.ToLower(strings.TrimPrefix(domain, "."))
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func cookieDomainMatches(host string, c cookieEntry) bool {
	if c.hostOnly {
		return host == c.domain
	}
	return domainMatches(host, c.domain)
}

func pathMatches(reqPath, cookiePath string) bool {
	if cookiePath == "" || cookiePath == "/" {
		return true
	}
	if reqPath == cookiePath {
		return true
	}
	return strings.HasPrefix(reqPath, cookiePath+"/")
}

// ParseCookieString parses "name=value; name2=value2" into unscoped cookies.
func ParseCookieString(s string) ([]*http.Cookie, error) {
	var out []*http.Cookie
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid cookie %q", part)
		}
		out = append(out, &http.Cookie{Name: name, Value: strings.TrimSpace(value)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no cookies in %q", s)
	}
	return out, nil
}

// ParseNetscapeFile reads cookies from a curl/Netscape cookie jar.
func ParseNetscapeFile(path string) ([]*http.Cookie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cookie file %s: %w", path, err)
	}

	var out []*http.Cookie
	for lineNo, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		httpOnly := false
		if strings.HasPrefix(line, "#HttpOnly_") {
			httpOnly = true
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			return nil, fmt.Errorf("%s:%d: expected 7 tab-separated fields", path, lineNo+1)
		}
		domain, includeSub, cookiePath, secure, expires, name := fields[0], fields[1], fields[2], fields[3], fields[4], fields[5]
		value := strings.Join(fields[6:], "\t")

		if strings.EqualFold(includeSub, "TRUE") {
			if !strings.HasPrefix(domain, ".") {
				domain = "." + domain
			}
		} else {
			domain = strings.TrimPrefix(domain, ".")
		}

		cookie := &http.Cookie{
			Domain:   domain,
			Path:     cookiePath,
			Secure:   strings.EqualFold(secure, "TRUE"),
			HttpOnly: httpOnly,
			Name:     name,
			Value:    value,
		}
		var unix int64
		if _, err := fmt.Sscanf(expires, "%d", &unix); err == nil && unix > 0 {
			cookie.Expires = time.Unix(unix, 0)
		}
		out = append(out, cookie)
	}
	return out, nil
}

// WriteNetscapeFile writes every cookie back in Netscape format. It is atomic
// (temp file + rename) and owner-only (0600).
func (s *CookieStore) WriteNetscapeFile(path string) error {
	var b strings.Builder
	b.WriteString("# Netscape HTTP Cookie File\n")
	b.WriteString("# https://curl.se/docs/http-cookies.html\n")
	b.WriteString("# Generated by aigc-cli. Edit at your own risk.\n\n")

	for _, c := range s.All() {
		domain := c.Domain
		includeSub := "TRUE"
		if !strings.HasPrefix(domain, ".") {
			includeSub = "FALSE"
		}
		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}
		expires := int64(0)
		if !c.Expires.IsZero() {
			expires = c.Expires.Unix()
		}
		prefix := ""
		if c.HttpOnly {
			prefix = "#HttpOnly_"
		}
		fmt.Fprintf(&b, "%s%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			prefix, sanitizeCookieField(domain), includeSub,
			sanitizeCookieField(c.Path), secure, expires,
			sanitizeCookieField(c.Name), sanitizeCookieField(c.Value))
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create cookie dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".aigc-cookies-*")
	if err != nil {
		return fmt.Errorf("create cookie temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod cookie file: %w", err)
	}
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return fmt.Errorf("write cookie file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close cookie file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace cookie file: %w", err)
	}
	return nil
}

func sanitizeCookieField(s string) string {
	return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s)
}
