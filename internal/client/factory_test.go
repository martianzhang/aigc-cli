package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestNewFromProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	tests := []struct {
		name         string
		p            *provider.EffectiveProvider
		wantAPIKey   string
		wantBaseURL  string
		wantProvider types.ProviderType
		wantProxy    string
	}{
		{
			name: "Ollama provider",
			p: &provider.EffectiveProvider{
				Type:    types.ProviderOllama,
				BaseURL: srv.URL,
				APIKey:  "ignored-key",
			},
			wantAPIKey:   "",
			wantBaseURL:  srv.URL + "/v1",
			wantProvider: types.ProviderOllama,
		},
		{
			name: "Anthropic provider",
			p: &provider.EffectiveProvider{
				Type:    types.ProviderAnthropic,
				BaseURL: srv.URL,
				APIKey:  "sk-ant-test",
			},
			wantAPIKey:   "sk-ant-test",
			wantBaseURL:  srv.URL + "/v1",
			wantProvider: types.ProviderAnthropic,
		},
		{
			name: "OpenAI provider",
			p: &provider.EffectiveProvider{
				Type:    types.ProviderOpenAI,
				BaseURL: srv.URL,
				APIKey:  "sk-openai-test",
			},
			wantAPIKey:   "sk-openai-test",
			wantBaseURL:  srv.URL + "/v1",
			wantProvider: types.ProviderOpenAI,
		},
		{
			name: "Unknown provider falls back to OpenAI",
			p: &provider.EffectiveProvider{
				Type:    types.ProviderGoogle,
				BaseURL: srv.URL,
				APIKey:  "sk-fallback-test",
			},
			wantAPIKey:   "sk-fallback-test",
			wantBaseURL:  srv.URL + "/v1",
			wantProvider: types.ProviderOpenAI,
		},
		{
			name: "Ollama with HTTPProxy",
			p: &provider.EffectiveProvider{
				Type:      types.ProviderOllama,
				BaseURL:   srv.URL,
				APIKey:    "",
				HTTPProxy: "http://proxy.example.com:8080",
			},
			wantAPIKey:   "",
			wantBaseURL:  srv.URL + "/v1",
			wantProvider: types.ProviderOllama,
			wantProxy:    "http://proxy.example.com:8080",
		},
		{
			name: "Anthropic with HTTPProxy",
			p: &provider.EffectiveProvider{
				Type:      types.ProviderAnthropic,
				BaseURL:   srv.URL,
				APIKey:    "sk-ant-proxy",
				HTTPProxy: "http://proxy.example.com:8080",
			},
			wantAPIKey:   "sk-ant-proxy",
			wantBaseURL:  srv.URL + "/v1",
			wantProvider: types.ProviderAnthropic,
			wantProxy:    "http://proxy.example.com:8080",
		},
		{
			name: "OpenAI with HTTPProxy",
			p: &provider.EffectiveProvider{
				Type:      types.ProviderOpenAI,
				BaseURL:   srv.URL,
				APIKey:    "sk-openai-proxy",
				HTTPProxy: "http://proxy.example.com:8080",
			},
			wantAPIKey:   "sk-openai-proxy",
			wantBaseURL:  srv.URL + "/v1",
			wantProvider: types.ProviderOpenAI,
			wantProxy:    "http://proxy.example.com:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewFromProvider(tt.p)

			if c.baseURL != tt.wantBaseURL {
				t.Errorf("baseURL = %q, want %q", c.baseURL, tt.wantBaseURL)
			}
			if c.apiKey != tt.wantAPIKey {
				t.Errorf("apiKey = %q, want %q", c.apiKey, tt.wantAPIKey)
			}
			if c.providerType != tt.wantProvider {
				t.Errorf("providerType = %q, want %q", c.providerType, tt.wantProvider)
			}
			if c.httpClient == nil {
				t.Fatal("httpClient is nil")
			}

			// Verify the client can make requests to the test server.
			req, err := http.NewRequest("GET", c.baseURL+"/models", nil)
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}
			resp, err := c.httpClient.Do(req)
			if err != nil {
				t.Fatalf("httpClient.Do() error = %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
			}

			// Verify proxy configuration.
			if tt.wantProxy != "" {
				tr, ok := c.httpClient.Transport.(*http.Transport)
				if !ok {
					t.Fatal("transport is not *http.Transport")
				}
				if tr.Proxy == nil {
					t.Fatal("transport.Proxy is nil")
				}
				// Proxy bypasses localhost/127.0.0.1, so verify with a non-localhost URL.
				proxyReq, _ := http.NewRequest("GET", "http://example.com", nil)
				proxyURL, err := tr.Proxy(proxyReq)
				if err != nil {
					t.Fatalf("Proxy() error = %v", err)
				}
				if proxyURL == nil {
					t.Fatal("Proxy() returned nil for non-localhost")
				}
				if proxyURL.String() != tt.wantProxy {
					t.Errorf("Proxy() = %q, want %q", proxyURL.String(), tt.wantProxy)
				}
			}
		})
	}
}

func TestNewFromProvider_apiKeyInRequestHeader(t *testing.T) {
	tests := []struct {
		name       string
		p          *provider.EffectiveProvider
		wantHeader string
		wantValue  string
	}{
		{
			name: "Ollama sends empty Bearer",
			p: &provider.EffectiveProvider{
				Type:   types.ProviderOllama,
				APIKey: "",
			},
			wantHeader: "Authorization",
			wantValue:  "Bearer", // Header.Get strips trailing space from "Bearer "
		},
		{
			name: "OpenAI sends Bearer token",
			p: &provider.EffectiveProvider{
				Type:   types.ProviderOpenAI,
				APIKey: "sk-openai-test",
			},
			wantHeader: "Authorization",
			wantValue:  "Bearer sk-openai-test",
		},
		{
			name: "Unknown provider sends Bearer token",
			p: &provider.EffectiveProvider{
				Type:   types.ProviderGoogle,
				APIKey: "sk-fallback-test",
			},
			wantHeader: "Authorization",
			wantValue:  "Bearer sk-fallback-test",
		},
		{
			name: "Anthropic sends x-api-key",
			p: &provider.EffectiveProvider{
				Type:   types.ProviderAnthropic,
				APIKey: "sk-ant-test",
			},
			wantHeader: "X-Api-Key",
			wantValue:  "sk-ant-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotValue string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotValue = r.Header.Get(tt.wantHeader)

				if tt.p.Type == types.ProviderAnthropic {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{
						"id": "msg_01",
						"type": "message",
						"role": "assistant",
						"model": "claude-3",
						"content": [{"type": "text", "text": "hi"}],
						"usage": {"input_tokens": 1, "output_tokens": 1}
					}`))
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{
					"id": "chatcmpl-1",
					"object": "chat.completion",
					"created": 1,
					"model": "gpt-4",
					"choices": [
						{"index": 0, "message": {"role": "assistant", "content": "hi"}, "finish_reason": "stop"}
					]
				}`))
			}))
			defer srv.Close()

			p := *tt.p
			p.BaseURL = srv.URL
			c := NewFromProvider(&p)

			_, err := c.ChatCompletion(&types.ChatRequest{
				Model:    "test-model",
				Messages: []types.ChatMessage{{Role: "user", Content: "hello"}},
			})
			if err != nil {
				t.Fatalf("ChatCompletion() error = %v", err)
			}

			if gotValue != tt.wantValue {
				t.Errorf("%s = %q, want %q", tt.wantHeader, gotValue, tt.wantValue)
			}
		})
	}
}
