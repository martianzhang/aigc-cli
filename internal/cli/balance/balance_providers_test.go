package balance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
)

// newBalanceTestClient returns a client pointed at a throwaway server running
// handler, so provider renderers can be exercised without real credentials.
func newBalanceTestClient(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return client.New("test-key", srv.URL, "")
}

func TestAPIMartSections(t *testing.T) {
	c := newBalanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/user/balance") {
			w.Write([]byte(`{"success":true,"remain_balance":0.28,"remain_credits":2.85,"used_balance":1.71,"used_credits":17.14}`))
			return
		}
		w.Write([]byte(`{"success":true,"unlimited_quota":true,"used_balance":0.03,"used_credits":0.31}`))
	})

	got := renderSections(apimartSections(c, "apimart"), "all")
	for _, want := range []string{"Token Balance (apimart):", "Status: Unlimited Quota", "User Balance (apimart):", "Remain Balance: $0.2800"} {
		if !strings.Contains(got, want) {
			t.Errorf("apimart sections = %q, want it to contain %q", got, want)
		}
	}
}

func TestOpenRouterSections_bothScopes(t *testing.T) {
	c := newBalanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/credits"):
			w.Write([]byte(`{"data":{"total_credits":100.5,"total_usage":25.75}}`))
		case strings.HasSuffix(r.URL.Path, "/key"):
			w.Write([]byte(`{"data":{"limit":100,"usage":25.5,"limit_remaining":74.5,"is_free_tier":false}}`))
		}
	})

	got := renderSections(openRouterSections(c, "or"), "all")
	for _, want := range []string{"Account Balance (or):", "Remain: $74.7500 (credits)", "Key Balance (or):", "Remain: $74.5000"} {
		if !strings.Contains(got, want) {
			t.Errorf("openrouter sections = %q, want it to contain %q", got, want)
		}
	}
}

func TestOpenRouterSections_keyOnly(t *testing.T) {
	c := newBalanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/credits") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":{"code":403,"message":"Only management keys can perform this operation"}}`))
			return
		}
		w.Write([]byte(`{"data":{"usage":1.5,"limit":null,"limit_remaining":null,"is_free_tier":true}}`))
	})

	got := renderSections(openRouterSections(c, "or"), "all")
	if strings.Contains(got, "Account Balance") {
		t.Errorf("openrouter sections = %q, want no account section on a non-management key", got)
	}
	for _, want := range []string{"Tier: free", "Key Balance (or):", "Limit: none", "management key"} {
		if !strings.Contains(got, want) {
			t.Errorf("openrouter sections = %q, want it to contain %q", got, want)
		}
	}
}

func TestRenderSections_scopeFilterAndFallback(t *testing.T) {
	sections := []balanceSection{
		{kind: "token", text: "TOKEN"},
		{kind: "user", text: "USER"},
	}

	if got := renderSections(sections, "all"); got != "TOKEN\n\nUSER" {
		t.Errorf("scope all = %q, want both sections", got)
	}
	if got := renderSections(sections, "token"); got != "TOKEN" {
		t.Errorf("scope token = %q, want only token", got)
	}
	if got := renderSections(sections, "user"); got != "USER" {
		t.Errorf("scope user = %q, want only user", got)
	}
	// A requested scope the provider does not expose falls back to all.
	single := []balanceSection{{kind: "user", text: "ONLY"}}
	if got := renderSections(single, "token"); got != "ONLY" {
		t.Errorf("scope token with only user section = %q, want fallback to all", got)
	}
}

func TestDeepSeekBalance(t *testing.T) {
	c := newBalanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/balance" {
			t.Errorf("path = %q, want /user/balance (host root, no /v1 prefix)", r.URL.Path)
		}
		w.Write([]byte(`{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"110.00","granted_balance":"10.00","topped_up_balance":"100.00"}]}`))
	})

	got := deepSeekBalance(c, "ds")
	if !strings.Contains(got, "CNY: total 110.00 (granted 10.00, topped-up 100.00)") {
		t.Errorf("deepSeekBalance() = %q", got)
	}
}

func TestMoonshotBalance(t *testing.T) {
	c := newBalanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/users/me/balance" {
			t.Errorf("path = %q, want /v1/users/me/balance", r.URL.Path)
		}
		w.Write([]byte(`{"code":0,"status":true,"data":{"available_balance":49.58894,"voucher_balance":46.58893,"cash_balance":3.00001}}`))
	})

	got := moonshotBalance(c, "kimi")
	for _, want := range []string{"Available: ¥49.58894", "Cash: ¥3.00001", "Voucher: ¥46.58893"} {
		if !strings.Contains(got, want) {
			t.Errorf("moonshotBalance() = %q, want it to contain %q", got, want)
		}
	}
}

func TestBalanceError_notAvailable(t *testing.T) {
	got := balanceError("Balance", "x", client.ErrNotSupported)
	if !strings.Contains(got, "not available") || !strings.Contains(got, "web console") {
		t.Errorf("balanceError(ErrNotSupported) = %q, want a console pointer", got)
	}
}
