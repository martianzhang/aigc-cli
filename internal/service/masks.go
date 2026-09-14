package service

import (
	"net/url"
	"regexp"
	"strings"
)

// urlMask replaces a hidden secret inside a URL. It must stay free of
// characters that url.URL.String percent-escapes (e.g. '*').
const urlMask = "REDACTED"

// secretParamNames lists query parameter names that carry credentials. It is
// shared by the parsed-URL and the fallback regex paths so both stay in sync.
const secretParamNames = `api_?key|apikey|key|access_?key|access_?token|auth_?token|token|client_?secret|secret|password|passwd|credential|auth|signature|sig|x-api-key`

var (
	// secretQueryParam matches query parameter names that carry credentials.
	secretQueryParam = regexp.MustCompile(`(?i)^(` + secretParamNames + `)$`)
	// secretPathSegment matches a whole path segment that looks like an API key.
	secretPathSegment = regexp.MustCompile(`(?i)^(sk-|fc-|hf_|AIza|ms-|AKLT)[A-Za-z0-9_\-]{8,}$`)
	// redactQueryFallback redacts secret query params in unparseable input.
	redactQueryFallback = regexp.MustCompile(`(?i)([?&](?:` + secretParamNames + `)=)([^&\s"']*)`)
	// redactUserinfoFallback redacts `user:pass@` in unparseable input.
	redactUserinfoFallback = regexp.MustCompile(`://([^/@\s:]+):([^/@\s]+)@`)
)

// MaskKey returns a masked form of an API key (last 4 chars), or "***" if too short.
func MaskKey(key string) string {
	if len(key) > 4 {
		return "..." + key[len(key)-4:]
	}
	return "***"
}

// RedactURLSecrets masks credentials and secret-looking parameters in a URL so
// the result is safe to print. Over-redaction is preferred over under-redaction.
func RedactURLSecrets(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return redactURLSecretsRaw(raw)
	}
	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass {
			u.User = url.UserPassword(u.User.Username(), urlMask)
		}
	}
	u.Path = redactPathSegments(u.Path)
	u.RawQuery = redactRawQuery(u.RawQuery)
	return u.String()
}

// redactPathSegments replaces path segments that look like API keys.
func redactPathSegments(path string) string {
	if path == "" {
		return path
	}
	segs := strings.Split(path, "/")
	for i, seg := range segs {
		if secretPathSegment.MatchString(seg) {
			segs[i] = urlMask
		}
	}
	return strings.Join(segs, "/")
}

// redactRawQuery masks values of secret-looking query parameters, preserving
// the original parameter order and encoding.
func redactRawQuery(rawQuery string) string {
	if rawQuery == "" {
		return rawQuery
	}
	parts := strings.Split(rawQuery, "&")
	for i, part := range parts {
		name, _, ok := strings.Cut(part, "=")
		if !ok || !secretQueryParam.MatchString(name) {
			continue
		}
		parts[i] = name + "=" + urlMask
	}
	return strings.Join(parts, "&")
}

// redactURLSecretsRaw is the best-effort fallback for input url.Parse rejects.
func redactURLSecretsRaw(raw string) string {
	out := redactUserinfoFallback.ReplaceAllString(raw, "://${1}:"+urlMask+"@")
	return redactQueryFallback.ReplaceAllString(out, "${1}"+urlMask)
}
