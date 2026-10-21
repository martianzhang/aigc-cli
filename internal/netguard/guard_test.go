package netguard

import (
	"net"
	"net/http"
	"testing"
	"time"
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
		if err := ValidatePublicURL(tc); err == nil {
			t.Errorf("ValidatePublicURL(%q) = nil, want error", tc)
		}
	}
}

func TestValidatePublicURL_RejectsLocalhost(t *testing.T) {
	// localhost resolves through the hosts file (or fails to resolve); either
	// outcome must be a rejection. No external network is used.
	if err := ValidatePublicURL("http://localhost/"); err == nil {
		t.Fatal("ValidatePublicURL(http://localhost/) = nil, want error")
	}
}

func TestValidatePublicURL_AcceptsPublicIP(t *testing.T) {
	if err := ValidatePublicURL("http://93.184.216.34/"); err != nil {
		t.Fatalf("ValidatePublicURL(public IP) = %v, want nil", err)
	}
}

func TestNewGuardedClientKeepsDefaultTransport(t *testing.T) {
	original := http.DefaultClient
	transport := &http.Transport{}
	http.DefaultClient = &http.Client{Transport: transport}
	t.Cleanup(func() { http.DefaultClient = original })

	client := NewGuardedClient(5 * time.Second)
	if client.Transport != transport {
		t.Error("NewGuardedClient did not reuse http.DefaultClient.Transport (proxy would be bypassed)")
	}
	if client.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", client.Timeout)
	}
	if client.CheckRedirect == nil {
		t.Error("CheckRedirect is nil, want per-hop validation")
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

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.1.1", "0.0.0.0", "224.0.0.1", "::1", "fe80::1", "fc00::1",
	}
	for _, s := range blocked {
		if ip := net.ParseIP(s); ip != nil && !isBlockedIP(ip) {
			t.Errorf("isBlockedIP(%s) = false, want true", s)
		}
	}
	public := []string{"8.8.8.8", "93.184.216.34", "1.1.1.1", "2606:4700::1111"}
	for _, s := range public {
		if ip := net.ParseIP(s); ip != nil && isBlockedIP(ip) {
			t.Errorf("isBlockedIP(%s) = true, want false", s)
		}
	}
}

func TestNewGuardedClient(t *testing.T) {
	c := NewGuardedClient(3 * time.Second)
	if c == http.DefaultClient {
		t.Error("NewGuardedClient should return a distinct client")
	}
	if c.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v, want 3s", c.Timeout)
	}
	if c.CheckRedirect == nil {
		t.Error("CheckRedirect = nil, want non-nil")
	}
}

func TestCheckRedirect(t *testing.T) {
	publicReq, err := http.NewRequest("GET", "http://93.184.216.34/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRedirect(publicReq, nil); err != nil {
		t.Errorf("checkRedirect(public) = %v, want nil", err)
	}

	blockedReq, err := http.NewRequest("GET", "http://127.0.0.1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRedirect(blockedReq, nil); err == nil {
		t.Error("checkRedirect(loopback) = nil, want error")
	}

	via := make([]*http.Request, 10)
	if err := checkRedirect(publicReq, via); err == nil {
		t.Error("checkRedirect(10 redirects) = nil, want error")
	}
}
