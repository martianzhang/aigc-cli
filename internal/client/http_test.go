package client

import (
	"net/http"
	"os"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

func TestConfigureDefaultClient_setsProxy(t *testing.T) {
	ConfigureDefaultClient("http://proxy.example.com:8080")

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	proxyURL, err := http.DefaultClient.Transport.(*http.Transport).Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() returned error: %v", err)
	}
	if proxyURL == nil {
		t.Fatal("Proxy() returned nil, expected proxy URL")
	}
	if proxyURL.String() != "http://proxy.example.com:8080" {
		t.Errorf("Proxy() = %q, want 'http://proxy.example.com:8080'", proxyURL.String())
	}
}

func TestConfigureDefaultClient_emptyProxy(t *testing.T) {
	// Save and restore env
	oldHTTPProxy := os.Getenv("HTTP_PROXY")
	oldHTTPSProxy := os.Getenv("HTTPS_PROXY")
	defer func() {
		os.Setenv("HTTP_PROXY", oldHTTPProxy)
		os.Setenv("HTTPS_PROXY", oldHTTPSProxy)
	}()
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")

	ConfigureDefaultClient("")

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	proxyURL, err := http.DefaultClient.Transport.(*http.Transport).Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() returned error: %v", err)
	}
	if proxyURL != nil {
		t.Logf("Proxy() = %q (may be from env)", proxyURL.String())
	}
}

func TestConfigureDefaultClient_invalidProxy(t *testing.T) {
	ConfigureDefaultClient("://invalid-proxy")
	// Should not panic; invalid URL is silently ignored, transport.Proxy remains nil
	tr, ok := http.DefaultClient.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Log("invalid proxy URL: transport.Proxy is nil (expected)")
		return
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	proxyURL, err := tr.Proxy(req)
	if err != nil {
		t.Logf("Proxy() error: %v (expected due to invalid proxy)", err)
	}
	if proxyURL != nil {
		t.Logf("Proxy() = %q", proxyURL.String())
	}
}

func TestConfigureDefaultClient_syncsDefaultTransport(t *testing.T) {
	provider.SetDirectMode(false)
	t.Cleanup(func() { provider.SetDirectMode(false) })

	ConfigureDefaultClient("http://proxy.example.com:8080")

	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("DefaultTransport is %T, want *http.Transport", http.DefaultTransport)
	}
	// Ad-hoc clients build &http.Client{} with no Transport and inherit this
	// one, so it must carry the same proxy as DefaultClient.
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() error: %v", err)
	}
	if u == nil || u.String() != "http://proxy.example.com:8080" {
		t.Errorf("DefaultTransport proxy = %v, want http://proxy.example.com:8080", u)
	}
}

func TestConfigureDefaultClient_directModeBypassesEnvProxy(t *testing.T) {
	// A proxy in the environment must not be picked up under --no-proxy.
	t.Setenv("HTTP_PROXY", "http://env-proxy:8080")
	t.Setenv("HTTPS_PROXY", "http://env-proxy:8080")
	provider.SetDirectMode(true)
	t.Cleanup(func() { provider.SetDirectMode(false) })

	// Even an explicit proxy URL must be ignored while direct mode is on.
	ConfigureDefaultClient("http://cli-proxy:8080")

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	for name, rt := range map[string]http.RoundTripper{
		"DefaultClient":    http.DefaultClient.Transport,
		"DefaultTransport": http.DefaultTransport,
	} {
		tr, ok := rt.(*http.Transport)
		if !ok {
			t.Fatalf("%s is %T, want *http.Transport", name, rt)
		}
		if tr.Proxy != nil {
			u, err := tr.Proxy(req)
			t.Errorf("%s uses proxy %v (err %v), want direct connection", name, u, err)
		}
	}
}
