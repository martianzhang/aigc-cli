package client

import (
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

// ConfigureDefaultClient sets the global http.DefaultClient's transport
// to use the given proxy URL. When proxyURL is empty, it falls back to
// HTTP_PROXY / HTTPS_PROXY / NO_PROXY environment variables.
// Local/loopback addresses bypass the proxy automatically.
//
// While --no-proxy is active (provider.SetDirectMode(true)), NewTransport
// returns a direct transport regardless of proxyURL, so both the default
// client and ad-hoc clients below ignore every proxy.
//
// Call this once at startup so that ALL HTTP requests in the application
// (including those using http.Get(), http.DefaultClient, image downloads,
// ideas search, etc.) respect the proxy configuration.
func ConfigureDefaultClient(proxyURL string) {
	transport := provider.NewTransport(proxyURL)
	http.DefaultClient = &http.Client{Transport: transport}
	// Ad-hoc clients (uploads, downloads, embeddings) are built as
	// &http.Client{} with no Transport, so they inherit DefaultTransport.
	// Keep it in sync or those requests would silently ignore --no-proxy
	// (and the configured proxy) by falling back to the environment.
	http.DefaultTransport = transport
}
