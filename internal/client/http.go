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
// Call this once at startup so that ALL HTTP requests in the application
// (including those using http.Get(), http.DefaultClient, image downloads,
// ideas search, etc.) respect the proxy configuration.
func ConfigureDefaultClient(proxyURL string) {
	http.DefaultClient = &http.Client{Transport: provider.NewTransport(proxyURL)}
}
