package balance

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// newBalanceTestClient returns a client pointed at a throwaway server running
// handler, so provider renderers can be exercised without real credentials.
func newBalanceTestClient(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return client.New("test-key", srv.URL, "")
}

func TestSomarkUsageSection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"code":0,"data":{"remaining_paid_pages":0,
			"remaining_free_pages_today":423,"remaining_free_pages_this_month":1923,
			"daily_free_limit":500,"monthly_free_limit":2000,
			"dashboard_url":"https://somark.tech/workbench"}}`)
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1", APIKey: "sk-test"}
	got := Query(p, "all")
	for _, want := range []string{
		"Usage (unknown):",
		"Remaining paid pages: 0",
		"Remaining free pages today: 423 / 500",
		"Remaining free pages this month: 1923 / 2000",
		"Console: https://somark.tech/workbench",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("somark balance = %q, want it to contain %q", got, want)
		}
	}
}

func TestSomarkUsageSection_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "not found")
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{Name: "oxr", Type: types.ProviderSomark, BaseURL: srv.URL + "/v1", APIKey: "dummy"}
	got := Query(p, "user")
	if !strings.Contains(got, "not available") {
		t.Errorf("balance = %q, want the console pointer", got)
	}
}

func TestOfoxBalanceSection(t *testing.T) {
	c := newBalanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/balance" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"is_active":true,"balance":9.32,"total":10,"used":0.6791,"currency":"USD"}`))
	})

	got := renderSections([]balanceSection{{kind: "user", text: ofoxBalance(c, "ofox")}}, "user")
	for _, want := range []string{"Balance (ofox):", "Remain: 9.3200 USD", "Used: 0.6791 USD", "Total: 10.0000 USD"} {
		if !strings.Contains(got, want) {
			t.Errorf("ofox balance = %q, want it to contain %q", got, want)
		}
	}
}

func TestOfoxRoutedByHost(t *testing.T) {
	// The OfoxAI host must select its dedicated renderer (its schema differs
	// from APIMart's), not the generic probe. The client points at a local
	// server so no real request leaves the test.
	c := newBalanceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"is_active":true,"balance":1.5,"currency":"USD"}`))
	})
	p := &provider.EffectiveProvider{BaseURL: "https://api.ofox.ai/v1"}

	sections, ok := providerSections(c, p, "ofox")
	if !ok {
		t.Fatal("providerSections did not recognize the OfoxAI host")
	}
	if len(sections) != 1 || !strings.Contains(sections[0].text, "Remain: 1.5000 USD") {
		t.Errorf("sections = %+v", sections)
	}
}

func TestGenericFallback_OpenAIBilling(t *testing.T) {
	// A new-api/one-api relay: APIMart-style endpoints 404, only the billing
	// pair answers.
	c := newBalanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/subscription"):
			w.Write([]byte(`{"object":"billing_subscription","hard_limit_usd":1000000}`))
		case strings.HasSuffix(r.URL.Path, "/usage"):
			w.Write([]byte(`{"object":"list","total_usage":1689.464}`))
		default:
			http.NotFound(w, r)
		}
	})

	got := genericBalance(c, "aibase", "all")
	for _, want := range []string{"Usage (aibase):", "Used: $16.8946 (last 30 days)", "Remaining: not exposed by this endpoint"} {
		if !strings.Contains(got, want) {
			t.Errorf("billing fallback = %q, want it to contain %q", got, want)
		}
	}
}

func TestGenericFallback_RealLimit(t *testing.T) {
	c := newBalanceTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/subscription"):
			w.Write([]byte(`{"object":"billing_subscription","hard_limit_usd":100}`))
		case strings.HasSuffix(r.URL.Path, "/usage"):
			w.Write([]byte(`{"object":"list","total_usage":2500}`))
		default:
			http.NotFound(w, r)
		}
	})

	got := genericBalance(c, "relay", "all")
	for _, want := range []string{"Used: $25.0000", "Limit: $100.0000", "Remain: $75.0000"} {
		if !strings.Contains(got, want) {
			t.Errorf("billing fallback = %q, want it to contain %q", got, want)
		}
	}
}

func TestGenericFallback_NoBillingAPI(t *testing.T) {
	c := newBalanceTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	})
	if got := genericBalance(c, "unknown", "all"); !strings.Contains(got, "not available") {
		t.Errorf("balance = %q, want the console pointer", got)
	}
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
