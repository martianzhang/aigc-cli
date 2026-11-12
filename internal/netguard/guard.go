// Package netguard provides SSRF protection for outbound HTTP fetches.
//
// It rejects URLs whose host does not resolve to a public address and
// re-validates every redirect hop, so a fetch of an attacker-influenced URL
// cannot be bounced onto loopback, private, link-local or cloud-metadata
// services. Both the knowledge-base fetcher and chat's web_fetch primitive use
// it so the guard stays a single implementation.
package netguard

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// NewGuardedClient returns a shallow copy of http.DefaultClient with the given
// timeout and a redirect hook that re-validates each hop. Copying
// http.DefaultClient keeps the app-wide transport — and with it the configured
// HTTP proxy — instead of bypassing it with a private client.
func NewGuardedClient(timeout time.Duration) *http.Client {
	return newGuardedClient(timeout, false)
}

// NewGuardedClientAllowLocal is NewGuardedClient with loopback addresses
// permitted. Media downloads use it so a local provider (Ollama, LocalAI) can
// serve assets from 127.0.0.1 while private/link-local/metadata addresses stay
// blocked.
func NewGuardedClientAllowLocal(timeout time.Duration) *http.Client {
	return newGuardedClient(timeout, true)
}

func newGuardedClient(timeout time.Duration, allowLoopback bool) *http.Client {
	c := *http.DefaultClient
	c.Timeout = timeout
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return checkRedirect(req, via, allowLoopback)
	}
	return &c
}

// checkRedirect re-validates every redirect target so a public URL cannot
// bounce the fetch onto a loopback or private address.
func checkRedirect(req *http.Request, via []*http.Request, allowLoopback bool) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return validateURLPolicy(req.URL.String(), net.LookupIP, allowLoopback)
}

// ValidatePublicURL rejects URLs whose host is not a public http/https address.
// It blocks loopback, private (RFC1918 / ULA fc00::/7), link-local
// (169.254.0.0/16 and fe80::/10), unspecified and multicast targets so a fetch
// cannot be used for SSRF against local or cloud-metadata services.
func ValidatePublicURL(rawURL string) error {
	return validateURL(rawURL, net.LookupIP)
}

// ValidatePublicURLAllowLocal is ValidatePublicURL but permits loopback hosts.
func ValidatePublicURLAllowLocal(rawURL string) error {
	return validateURLPolicy(rawURL, net.LookupIP, true)
}

// validateURL implements ValidatePublicURL with an injectable resolver so the
// hostname paths can be tested without network access.
func validateURL(rawURL string, lookup func(host string) ([]net.IP, error)) error {
	return validateURLPolicy(rawURL, lookup, false)
}

func validateURLPolicy(rawURL string, lookup func(host string) ([]net.IP, error), allowLoopback bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL %q: unsupported scheme %q", rawURL, u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL %q: missing host", rawURL)
	}
	// Literal IPs skip DNS resolution entirely.
	if ip := net.ParseIP(host); ip != nil {
		return checkIP(rawURL, ip, allowLoopback)
	}
	ips, err := lookup(host)
	if err != nil {
		return fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("host %q has no addresses", host)
	}
	// Every answer must be public: a hostname resolving to a mix of public
	// and private addresses (DNS rebinding, split-horizon DNS) is rejected.
	for _, ip := range ips {
		if err := checkIP(rawURL, ip, allowLoopback); err != nil {
			return err
		}
	}
	return nil
}

// checkIP rejects a single resolved address that is not publicly routable.
func checkIP(rawURL string, ip net.IP, allowLoopback bool) error {
	if allowLoopback && ip.IsLoopback() {
		return nil
	}
	if isBlockedIP(ip) {
		return fmt.Errorf("URL %q resolves to non-public address %s", rawURL, ip)
	}
	return nil
}

// isBlockedIP reports whether ip is a loopback, private, link-local,
// unspecified or multicast address.
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}
