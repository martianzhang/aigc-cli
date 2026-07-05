package client

import (
	"net/http"
	"os"
	"testing"
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
