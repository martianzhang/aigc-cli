package balance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestProviderLabel(t *testing.T) {
	if got := providerLabel(&provider.EffectiveProvider{Name: "siliconflow"}); got != "siliconflow" {
		t.Errorf("providerLabel(named) = %q, want siliconflow", got)
	}
	got := providerLabel(&provider.EffectiveProvider{ProviderType: provider.APIMart})
	if got != provider.APIMart.String() {
		t.Errorf("providerLabel(typed) = %q, want %q", got, provider.APIMart.String())
	}
}

func TestCollectProviders(t *testing.T) {
	t.Run("none configured", func(t *testing.T) {
		if got := collectProviders(Deps{}); got != nil {
			t.Errorf("collectProviders() = %v, want nil", got)
		}
	})

	t.Run("explicit provider", func(t *testing.T) {
		want := &provider.EffectiveProvider{Name: "p1"}
		got := collectProviders(Deps{
			ProviderSet:     true,
			Provider:        "p1",
			ResolveProvider: func(string) *provider.EffectiveProvider { return want },
		})
		if len(got) != 1 || got[0] != want {
			t.Errorf("collectProviders() = %v, want [p1]", got)
		}
	})
}

func TestGetTextExplicitKeylessProviderFailsFast(t *testing.T) {
	d := Deps{
		ProviderSet: true,
		Provider:    "nokey",
		ResolveProvider: func(string) *provider.EffectiveProvider {
			return &provider.EffectiveProvider{Name: "nokey", BaseURL: "http://balance-no-key.invalid/v1", Type: types.ProviderOpenAI}
		},
	}

	_, err := GetText(d, "token")
	if err == nil {
		t.Fatal("GetText() = nil, want missing-key error")
	}
	if !strings.Contains(err.Error(), "no API key") {
		t.Errorf("GetText() error = %q, want it to report the missing key", err)
	}
	if strings.Contains(err.Error(), "API returned status") || strings.Contains(err.Error(), "error —") {
		t.Errorf("GetText() error = %q, want no HTTP attempt before the guard", err)
	}
	if strings.Contains(err.Error(), "no providers configured") {
		t.Errorf("GetText() error = %q, want the explicit-provider guard, not the fan-out fallback", err)
	}
}

func TestGetTextKeylessLoopbackQueries(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"remain_balance":1.5,"remain_credits":2.5,"used_balance":0.5,"used_credits":0.25}`))
	}))
	defer srv.Close()

	ep := &provider.EffectiveProvider{Name: "loop", BaseURL: srv.URL, Type: types.ProviderOpenAI}
	d := Deps{
		ProviderSet:     true,
		Provider:        "loop",
		ResolveProvider: func(string) *provider.EffectiveProvider { return ep },
	}

	text, err := GetText(d, "token")
	if err != nil {
		t.Fatalf("GetText() error = %v, want nil", err)
	}
	if !strings.Contains(text, "Token Balance (loop)") {
		t.Errorf("GetText() = %q, want loop provider report", text)
	}
	if got := atomic.LoadInt32(&hits); got < 1 {
		t.Errorf("server hits = %d, want >= 1", got)
	}
}
