package search

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// searchCacheDir returns the per-provider cache directory. AIGC_CLI_CACHE_DIR
// overrides the OS cache root (used by tests and sandboxed runs).
func searchCacheDir(provider string) string {
	root := os.Getenv("AIGC_CLI_CACHE_DIR")
	if root == "" {
		if dir, err := os.UserCacheDir(); err == nil {
			root = dir
		} else {
			root = os.TempDir()
		}
	}
	return filepath.Join(root, "aigc-cli", "search", provider)
}

// cacheKey hashes a request URL after removing the API key, so credentials
// never become part of a cache filename or its contents.
func cacheKey(rawURL string) string {
	normalized := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		values := u.Query()
		values.Del("key")
		u.RawQuery = values.Encode()
		normalized = u.String()
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// readSearchCache returns a cached response younger than ttl.
func readSearchCache(provider, key string, ttl time.Duration) (json.RawMessage, bool) {
	path := filepath.Join(searchCacheDir(provider), key+".json")
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > ttl {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return json.RawMessage(data), true
}

// writeSearchCache stores a successful response best-effort; a cache failure
// must never fail the search itself.
func writeSearchCache(provider, key string, raw json.RawMessage) {
	dir := searchCacheDir(provider)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, key+".json"), raw, 0o644)
}
