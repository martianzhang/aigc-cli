package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newBillingTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New("test-key", srv.URL+"/v1", "")
}

func TestGetOfoxBalance(t *testing.T) {
	c := newBillingTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user/balance" {
			t.Errorf("path = %q, want /v1/user/balance", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Write([]byte(`{"is_active":true,"balance":9.32,"total":10,"used":0.6791,"currency":"USD"}`))
	})

	bal, err := c.GetOfoxBalance()
	if err != nil {
		t.Fatalf("GetOfoxBalance: %v", err)
	}
	if !bal.IsActive || bal.Balance != 9.32 || bal.Used != 0.6791 || bal.Currency != "USD" {
		t.Errorf("balance = %+v", bal)
	}
}

func TestGetOpenAIBilling(t *testing.T) {
	c := newBillingTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/dashboard/billing/subscription":
			w.Write([]byte(`{"object":"billing_subscription","hard_limit_usd":1000000,"soft_limit_usd":1000000}`))
		case "/v1/dashboard/billing/usage":
			if !strings.Contains(r.URL.RawQuery, "start_date=") || !strings.Contains(r.URL.RawQuery, "end_date=") {
				t.Errorf("usage query = %q, want a date range", r.URL.RawQuery)
			}
			w.Write([]byte(`{"object":"list","total_usage":1689.464}`))
		default:
			http.NotFound(w, r)
		}
	})

	subscription, usage, err := c.GetOpenAIBilling()
	if err != nil {
		t.Fatalf("GetOpenAIBilling: %v", err)
	}
	if subscription == nil || subscription.HardLimitUSD != 1000000 {
		t.Errorf("subscription = %+v", subscription)
	}
	if usage == nil || usage.TotalUsage != 1689.464 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestGetOpenAIBilling_usageOnly(t *testing.T) {
	c := newBillingTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/usage") {
			w.Write([]byte(`{"object":"list","total_usage":12.34}`))
			return
		}
		http.NotFound(w, r)
	})

	subscription, usage, err := c.GetOpenAIBilling()
	if err != nil {
		t.Fatalf("GetOpenAIBilling: %v", err)
	}
	if subscription != nil {
		t.Errorf("subscription = %+v, want nil", subscription)
	}
	if usage == nil || usage.TotalUsage != 12.34 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestGetOpenAIBilling_unavailable(t *testing.T) {
	c := newBillingTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	})
	subscription, usage, err := c.GetOpenAIBilling()
	if err == nil {
		t.Fatal("expected an error when neither endpoint answers")
	}
	if subscription != nil || usage != nil {
		t.Errorf("subscription = %v, usage = %v, want both nil", subscription, usage)
	}
}
