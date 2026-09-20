package knowledge

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestValidatePublicURL_RejectsBlockedLiteralIPs(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/",
		"http://127.0.0.1:8080/path",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/",
		"http://192.168.1.1/",
		"http://172.16.5.5/",
		"http://[::1]/",
		"http://[fc00::1]/",
		"http://[fe80::1]/",
		"http://0.0.0.0/",
		"http://239.1.2.3/",
		"file:///etc/passwd",
		"ftp://example.com/",
	}
	for _, tc := range cases {
		if err := validatePublicURL(tc); err == nil {
			t.Errorf("validatePublicURL(%q) = nil, want error", tc)
		}
	}
}

func TestValidatePublicURL_RejectsLocalhost(t *testing.T) {
	// localhost resolves through the hosts file (or fails to resolve); either
	// outcome must be a rejection. No external network is used.
	if err := validatePublicURL("http://localhost/"); err == nil {
		t.Fatal("validatePublicURL(http://localhost/) = nil, want error")
	}
}

func TestValidatePublicURL_AcceptsPublicIP(t *testing.T) {
	if err := validatePublicURL("http://93.184.216.34/"); err != nil {
		t.Fatalf("validatePublicURL(public IP) = %v, want nil", err)
	}
}

func TestValidateURL_PublicHostname(t *testing.T) {
	lookup := func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	if err := validateURL("https://example.com/page", lookup); err != nil {
		t.Fatalf("validateURL(example.com) = %v, want nil", err)
	}
}

func TestValidateURL_RejectsHostnameResolvingToPrivateOrMixedIPs(t *testing.T) {
	private := func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.1.2.3")}, nil
	}
	if err := validateURL("https://internal.example/", private); err == nil {
		t.Error("hostname resolving to a private IP should be rejected")
	}

	mixed := func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("169.254.169.254")}, nil
	}
	if err := validateURL("https://mixed.example/", mixed); err == nil {
		t.Error("any blocked address in a DNS answer should reject the URL")
	}

	failing := func(host string) ([]net.IP, error) {
		return nil, net.UnknownNetworkError("no such host")
	}
	if err := validateURL("https://unresolvable.example/", failing); err == nil {
		t.Error("resolution failure should reject the URL")
	}
}

func TestFetchURL_RejectsLoopbackServer(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("<html><body>secret</body></html>"))
	}))
	defer srv.Close()

	if _, err := FetchURL(srv.URL); err == nil {
		t.Fatal("FetchURL on a loopback httptest server should be rejected")
	}
	if hits.Load() != 0 {
		t.Errorf("guard rejected the URL but the server served %d request(s)", hits.Load())
	}
}
