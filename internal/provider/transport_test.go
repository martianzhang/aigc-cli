package provider

import (
	"net/http"
	"net/url"
	"testing"
)

func TestNewTransportKeepsStdlibDefaults(t *testing.T) {
	tr := NewTransport("")
	if tr.DialContext == nil {
		t.Error("DialContext is nil: connection attempts have no timeout")
	}
	if tr.TLSHandshakeTimeout == 0 {
		t.Error("TLSHandshakeTimeout is zero")
	}
	if tr.IdleConnTimeout == 0 {
		t.Error("IdleConnTimeout is zero")
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 is false")
	}
	if tr.MaxIdleConns == 0 {
		t.Error("MaxIdleConns is zero")
	}
}

func TestNewTransportCachesByProxy(t *testing.T) {
	a := NewTransport("http://proxy.example:8080")
	b := NewTransport("http://proxy.example:8080")
	if a != b {
		t.Error("NewTransport must return the cached transport for the same proxy")
	}
	if a.Proxy == nil {
		t.Error("configured transport must set a Proxy function")
	}
}

func TestProxyFuncBypassesLoopback(t *testing.T) {
	proxy := ProxyFunc("http://proxy.example:8080")
	req := &http.Request{URL: &url.URL{Scheme: "http", Host: "127.0.0.1:11434"}}
	if got, err := proxy(req); err != nil || got != nil {
		t.Fatalf("loopback proxy = (%v, %v), want (nil, nil)", got, err)
	}

	req.URL = &url.URL{Scheme: "https", Host: "api.example.com"}
	got, err := proxy(req)
	if err != nil {
		t.Fatalf("proxy() error = %v", err)
	}
	if got == nil || got.Host != "proxy.example:8080" {
		t.Fatalf("external proxy = %v, want proxy.example:8080", got)
	}
}

func TestDirectTransportHasNoProxy(t *testing.T) {
	SetDirectMode(true)
	defer SetDirectMode(false)

	tr := NewDirectTransport()
	if tr.Proxy != nil {
		t.Error("direct transport must have a nil Proxy function")
	}
	if tr.DialContext == nil || tr.TLSHandshakeTimeout == 0 {
		t.Error("direct transport must keep the stdlib timeout defaults")
	}
}
