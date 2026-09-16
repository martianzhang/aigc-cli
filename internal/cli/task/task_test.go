package task

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
)

func TestQueryText_nonAPIMart(t *testing.T) {
	t.Run("unknown provider", func(t *testing.T) {
		_, err := QueryText(Deps{}, "task_1")
		if err == nil || !strings.Contains(err.Error(), "only supported on APIMart") {
			t.Fatalf("got err=%v, want APIMart-only error", err)
		}
	})

	t.Run("openrouter hint", func(t *testing.T) {
		d := Deps{
			ResolveProvider: func(string) *provider.EffectiveProvider {
				return &provider.EffectiveProvider{
					BaseURL:      "https://openrouter.ai/api/v1",
					ProviderType: provider.OpenRouter,
				}
			},
		}
		_, err := QueryText(d, "task_1")
		if err == nil || !strings.Contains(err.Error(), "--job-id") {
			t.Fatalf("got err=%v, want OpenRouter hint", err)
		}
	})
}

func TestQueryText_apimartMissingKeyFailsFast(t *testing.T) {
	d := Deps{
		ResolveProvider: func(string) *provider.EffectiveProvider {
			return &provider.EffectiveProvider{
				Name:         "apimart",
				BaseURL:      "https://api.apimart.ai",
				ProviderType: provider.APIMart,
			}
		},
	}

	_, err := QueryText(d, "task_1")
	if err == nil {
		t.Fatal("QueryText() = nil, want missing-key error")
	}
	if !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("got err=%v, want missing API key error", err)
	}
}

func TestQueryText_apimartUsesProviderCredentials(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"id":"task_1","status":"completed","progress":100}}`))
	}))
	defer srv.Close()

	d := Deps{
		ResolveProvider: func(string) *provider.EffectiveProvider {
			return &provider.EffectiveProvider{
				Name:         "apimart",
				APIKey:       "test-key",
				BaseURL:      srv.URL,
				ProviderType: provider.APIMart,
			}
		},
	}

	text, err := QueryText(d, "task_1")
	if err != nil {
		t.Fatalf("QueryText() error = %v, want nil", err)
	}
	if !strings.Contains(text, "Status: completed") {
		t.Errorf("QueryText() = %q, want it to contain Status: completed", text)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want the provider key to be used", gotAuth)
	}
	if gotPath != "/v1/tasks/task_1" {
		t.Errorf("request path = %q, want /v1/tasks/task_1", gotPath)
	}
}
