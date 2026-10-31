package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestNewFromProviderAppliesExplicitTimeout(t *testing.T) {
	p := &provider.EffectiveProvider{
		Type:    types.ProviderOpenAI,
		BaseURL: "https://api.example.com",
		Timeout: 45 * time.Second,
	}
	c := NewFromProvider(p)
	if got := c.httpClient.Timeout; got != 45*time.Second {
		t.Errorf("httpClient.Timeout = %v, want 45s", got)
	}

	p.Timeout = 0
	c = NewFromProvider(p)
	if got := c.httpClient.Timeout; got != DefaultTimeout {
		t.Errorf("httpClient.Timeout = %v, want default %v", got, DefaultTimeout)
	}
}

func TestUploadClientUsesConfiguredTimeout(t *testing.T) {
	c := NewWithProvider("k", "https://api.example.com", "", types.ProviderOpenAI)

	c.SetTimeout(42 * time.Second)
	if got := c.uploadClient().Timeout; got != 42*time.Second {
		t.Errorf("upload timeout = %v, want 42s", got)
	}

	// No configured timeout falls back to the upload-specific budget.
	c.SetTimeout(0)
	if got := c.uploadClient().Timeout; got != uploadTimeout {
		t.Errorf("upload timeout = %v, want fallback %v", got, uploadTimeout)
	}
}

func TestOpenRouterImageHonorsConfiguredTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"aGk="}]}`))
	}))
	defer srv.Close()

	c := NewWithProvider("k", srv.URL, "", types.ProviderOpenAI)
	c.SetTimeout(50 * time.Millisecond)

	_, err := c.OpenRouterDedicatedImage(&types.GenerateRequest{
		Model:  "openai/gpt-image-2.5-flare",
		Prompt: "p",
	})
	if err == nil {
		t.Fatal("OpenRouterDedicatedImage() error = nil, want timeout error")
	}
}
