package knowledge

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// maxFetchBytes caps how much of a response body FetchURL reads into memory.
const maxFetchBytes = 10 * 1024 * 1024

// newKBClient returns a shallow copy of http.DefaultClient with the given
// timeout. Copying http.DefaultClient keeps the app-wide transport — and with
// it the configured HTTP proxy — instead of bypassing it with a private client.
func newKBClient(timeout time.Duration) *http.Client {
	c := *http.DefaultClient
	c.Timeout = timeout
	c.CheckRedirect = checkRedirect
	return &c
}

// checkRedirect re-validates every redirect target so a public URL cannot
// bounce the fetch onto a loopback or private address.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return validatePublicURL(req.URL.String())
}

// validatePublicURL rejects URLs whose host is not a public http/https address.
// It blocks loopback, private (RFC1918 / ULA fc00::/7), link-local
// (169.254.0.0/16 and fe80::/10), unspecified and multicast targets so a KB
// fetch cannot be used for SSRF against local or cloud-metadata services.
func validatePublicURL(rawURL string) error {
	return validateURL(rawURL, net.LookupIP)
}

// validateURL implements validatePublicURL with an injectable resolver so the
// hostname paths can be tested without network access.
func validateURL(rawURL string, lookup func(host string) ([]net.IP, error)) error {
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
		return checkIP(rawURL, ip)
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
		if err := checkIP(rawURL, ip); err != nil {
			return err
		}
	}
	return nil
}

// checkIP rejects a single resolved address that is not publicly routable.
func checkIP(rawURL string, ip net.IP) error {
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
