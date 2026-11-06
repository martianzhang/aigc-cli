package search

import (
	"net/http"
	"sync"
	"time"
)

// DefaultTimeout bounds one provider request when the user has not configured
// a top-level `timeout`. Callers (cmd/root.go) override it from
// --timeout / config `timeout`.
const DefaultTimeout = 30 * time.Second

var requestTimeout struct {
	mu sync.RWMutex
	d  time.Duration
}

// SetDefaultTimeout configures the per-request timeout for every search and
// fetch provider. d <= 0 restores DefaultTimeout.
func SetDefaultTimeout(d time.Duration) {
	requestTimeout.mu.Lock()
	requestTimeout.d = d
	requestTimeout.mu.Unlock()
}

// RequestTimeout returns the configured provider timeout, or DefaultTimeout.
func RequestTimeout() time.Duration {
	requestTimeout.mu.RLock()
	d := requestTimeout.d
	requestTimeout.mu.RUnlock()
	if d > 0 {
		return d
	}
	return DefaultTimeout
}

// httpClient returns a copy of http.DefaultClient with the configured timeout.
// Copying keeps the app-wide transport (proxy, TLS settings) intact.
func httpClient() *http.Client {
	c := *http.DefaultClient
	c.Timeout = RequestTimeout()
	return &c
}
