// Package webfetch is the shared HTTP + readability + Markdown core used by the
// knowledge base, chat's fetch primitive, and the CLI `fetch` command.
package webfetch

import (
	"fmt"
	"net/http"
	"net/textproto"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// DefaultUserAgent is a full browser User-Agent. Many sites return a JS
// redirect or anti-bot stub for non-browser agents, which yields empty readable
// content even though the HTTP request succeeded.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// DefaultAccept mirrors the broad Accept header used by the built-in fetcher.
const DefaultAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.5"

// DefaultAcceptLanguage favors Chinese then English.
const DefaultAcceptLanguage = "zh-CN,zh;q=0.9,en;q=0.8"

// Header is one user-supplied HTTP header. An empty Value removes the default
// header of the same name (curl's `-H "Name:"` behavior).
type Header struct {
	Name  string
	Value string
}

// ParseHeader parses one `-H` value: "Name: value" or "Name:" to remove.
// CR/LF and invalid field names/values are rejected to prevent header injection.
func ParseHeader(line string) (Header, error) {
	name, value, ok := strings.Cut(line, ":")
	if !ok {
		return Header{}, fmt.Errorf("header %q: expected \"Name: value\"", line)
	}
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if name == "" {
		return Header{}, fmt.Errorf("header %q: name is required", line)
	}
	if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
		return Header{}, fmt.Errorf("header %q: CR/LF is not allowed", line)
	}
	if !httpguts.ValidHeaderFieldName(name) {
		return Header{}, fmt.Errorf("header %q: invalid name", line)
	}
	if value != "" && !httpguts.ValidHeaderFieldValue(value) {
		return Header{}, fmt.Errorf("header %q: invalid value", line)
	}
	return Header{Name: name, Value: value}, nil
}

// ParseHeaders parses repeated -H values.
func ParseHeaders(lines []string) ([]Header, error) {
	headers := make([]Header, 0, len(lines))
	for _, line := range lines {
		h, err := ParseHeader(line)
		if err != nil {
			return nil, err
		}
		headers = append(headers, h)
	}
	return headers, nil
}

// EffectiveHeaders merges user headers over the built-in defaults.
func EffectiveHeaders(user []Header) []Header {
	return EffectiveHeadersWithDefaults([]Header{
		{Name: "User-Agent", Value: DefaultUserAgent},
		{Name: "Accept", Value: DefaultAccept},
		{Name: "Accept-Language", Value: DefaultAcceptLanguage},
	}, user)
}

// EffectiveHeadersWithDefaults merges user headers over caller-provided
// defaults. Later user headers win; an empty value removes the header. The
// result preserves the default ordering and appends user-only headers in input
// order.
func EffectiveHeadersWithDefaults(defaults, user []Header) []Header {
	canonical := func(name string) string { return textproto.CanonicalMIMEHeaderKey(name) }
	overrides := make(map[string]Header, len(user))
	for _, h := range user {
		overrides[canonical(h.Name)] = Header{Name: canonical(h.Name), Value: h.Value}
	}

	out := make([]Header, 0, len(defaults)+len(user))
	seen := make(map[string]bool, len(defaults)+len(user))
	for _, h := range defaults {
		key := canonical(h.Name)
		seen[key] = true
		if override, ok := overrides[key]; ok {
			if override.Value != "" {
				out = append(out, override)
			}
			continue
		}
		out = append(out, h)
	}
	for _, h := range user {
		key := canonical(h.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		if h.Value != "" {
			out = append(out, Header{Name: key, Value: h.Value})
		}
	}
	return out
}

// ApplyHeaders sets the given headers on req. It is exported so previews and
// real requests stay byte-for-byte aligned.
func ApplyHeaders(req *http.Request, headers []Header) {
	for _, h := range headers {
		req.Header.Set(h.Name, h.Value)
	}
}
