package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// captureStdout runs fn and returns everything written to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

func TestFormatPrice_perGeneration(t *testing.T) {
	m := types.MarketplaceModel{
		Pricing: types.MarketplacePricing{
			HasPrice:      true,
			StartingPrice: 0.006,
			PriceUnit:     "/次",
			BillingType:   "per_generation",
		},
	}
	if got, want := m.FormatPrice(), "$0.0060/次"; got != want {
		t.Errorf("FormatPrice() = %q, want %q", got, want)
	}
}

func TestFormatPrice_perToken(t *testing.T) {
	m := types.MarketplaceModel{
		Pricing: types.MarketplacePricing{
			HasPrice:      true,
			StartingPrice: 4.0,
			PriceUnit:     "/1K tokens",
			BillingType:   "per_token",
		},
	}
	if got, want := m.FormatPrice(), "$4.0000/1K tokens"; got != want {
		t.Errorf("FormatPrice() = %q, want %q", got, want)
	}
}

func TestFormatPrice_noPrice(t *testing.T) {
	m := types.MarketplaceModel{Pricing: types.MarketplacePricing{HasPrice: false}}
	if got, want := m.FormatPrice(), "—"; got != want {
		t.Errorf("FormatPrice() = %q, want %q", got, want)
	}
}

func TestFormatPrice_emptyUnit(t *testing.T) {
	m := types.MarketplaceModel{
		Pricing: types.MarketplacePricing{
			HasPrice:      true,
			StartingPrice: 0.01,
			PriceUnit:     "",
			BillingType:   "per_generation",
		},
	}
	if got, want := m.FormatPrice(), "$0.0100/次"; got != want {
		t.Errorf("FormatPrice() = %q, want %q", got, want)
	}
}

func TestMainDomain(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "https://apimart.ai"},
		{"https://api.apimart.ai", "https://apimart.ai"},
		{"https://api.apimart.ai/v1", "https://apimart.ai"},
		{"https://custom.api.com", "https://custom.api.com"},
	}
	for _, tc := range tests {
		if got := mainDomain(tc.in); got != tc.want {
			t.Errorf("mainDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsHTML(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"html", []byte("<html><body>blocked</body></html>"), true},
		{"html leading whitespace", []byte("  \n\t<html>"), true},
		{"json object", []byte(`{"data":[]}`), false},
		{"json array", []byte(`[{"id":"test"}]`), false},
		{"empty", nil, false},
		{"plain text", []byte("OK"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHTML(tc.in); got != tc.want {
				t.Errorf("isHTML(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestOpenRouterModelsLocalServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": [
				{
					"id": "test/model",
					"name": "Test Model",
					"architecture": {
						"input_modalities": ["text"],
						"output_modalities": ["image"]
					},
					"supported_parameters": {
						"n": {"type": "range", "min": 1, "max": 4},
						"seed": {"type": "boolean"}
					},
					"supports_streaming": false
				}
			]
		}`))
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{BaseURL: srv.URL, ProviderType: provider.OpenRouter}
	if err := runModelsOpenRouterDiscovery(p, "image"); err != nil {
		t.Fatalf("runModelsOpenRouterDiscovery() error = %v", err)
	}
}

func TestRequireAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		p       *provider.EffectiveProvider
		wantErr bool
	}{
		{"empty key remote", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, true},
		{"key set", &provider.EffectiveProvider{APIKey: "sk-x", BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, false},
		{"nil provider", nil, false},
		{"ollama", &provider.EffectiveProvider{Type: types.ProviderOllama, BaseURL: "https://ollama.example.com"}, false},
		{"local onnx", &provider.EffectiveProvider{Type: types.ProviderLocal}, false},
		{"localhost exempt", &provider.EffectiveProvider{BaseURL: "http://localhost:1234/v1", Type: types.ProviderOpenAI}, false},
		{"127.0.0.1 exempt", &provider.EffectiveProvider{BaseURL: "http://127.0.0.1:1234/v1", Type: types.ProviderOpenAI}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := options.RequireAPIKey("models", tc.p, nil)
			if tc.wantErr && err == nil {
				t.Fatalf("RequireAPIKey() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("RequireAPIKey() = %v, want nil", err)
			}
		})
	}
}

func TestRequireAPIKeyErrorListsProviders(t *testing.T) {
	orig := d
	d = Deps{Providers: map[string]*types.NamedProvider{
		"zeta":  {APIKey: "k"},
		"alpha": {APIKey: "k"},
		"blank": {},
	}}
	defer func() { d = orig }()

	err := options.RequireAPIKey("models", &provider.EffectiveProvider{BaseURL: "https://api.example.com/v1", Type: types.ProviderOpenAI}, d.Providers)
	if err == nil {
		t.Fatal("RequireAPIKey() = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "(configured with keys: alpha, zeta)") {
		t.Errorf("error %q should list providers alphabetically and omit blank keys", msg)
	}
	if strings.Contains(msg, "blank") {
		t.Errorf("error %q should omit providers without keys", msg)
	}
}

func TestRunModelsOpenAINoRequestWithoutKey(t *testing.T) {
	orig := d
	d = Deps{}
	defer func() { d = orig }()

	p := &provider.EffectiveProvider{BaseURL: "http://models-no-key.invalid/v1", Type: types.ProviderOpenAI}
	err := runModelsOpenAI(p)
	if err == nil {
		t.Fatal("runModelsOpenAI() = nil, want missing-key error")
	}
	if !strings.Contains(err.Error(), "no API key") {
		t.Errorf("runModelsOpenAI() error = %q, want it to report the missing key", err)
	}
	if strings.Contains(err.Error(), "failed to list models") {
		t.Errorf("runModelsOpenAI() error = %q, want no HTTP attempt before the guard", err)
	}
}

func TestRunModelsDetailNoRequestWithoutKey(t *testing.T) {
	orig := d
	d = Deps{}
	defer func() { d = orig }()

	p := &provider.EffectiveProvider{BaseURL: "http://models-no-key.invalid/v1", Type: types.ProviderOpenAI}
	err := runModelsDetail("m", p)
	if err == nil {
		t.Fatal("runModelsDetail() = nil, want missing-key error")
	}
	if !strings.Contains(err.Error(), "no API key") {
		t.Errorf("runModelsDetail() error = %q, want it to report the missing key", err)
	}
	if strings.Contains(err.Error(), "failed to get model") {
		t.Errorf("runModelsDetail() error = %q, want no HTTP attempt before the guard", err)
	}
}

func TestRunModelsOpenRouterDetailLocalServer(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": {
				"id": "openai/gpt-6-luna",
				"name": "GPT-6 Luna",
				"description": "test model",
				"context_length": 128000,
				"architecture": {"input_modalities": ["text"], "output_modalities": ["text"]},
				"supported_parameters": ["tools", "temperature", "top_p"],
				"top_provider": {"context_length": 128000, "max_completion_tokens": 16384},
				"pricing": {"prompt": "0.0000025", "completion": "0.00001"}
			}
		}`))
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{
		BaseURL:      srv.URL + "/api/v1",
		ProviderType: provider.OpenRouter,
		Type:         types.ProviderOpenAI,
	}
	if err := runModelsOpenRouterDetail("openai/gpt-6-luna", p); err != nil {
		t.Fatalf("runModelsOpenRouterDetail() error = %v", err)
	}
	if want := "/api/v1/model/openai/gpt-6-luna"; gotPath != want {
		t.Errorf("request path = %q, want singular %q", gotPath, want)
	}
}

func TestIndentJSONPreservesArbitraryShape(t *testing.T) {
	raw := json.RawMessage(`{"data":{"supported_parameters":["tools"],"nested":{"a":[1,2]}}}`)
	got, err := indentJSON(raw)
	if err != nil {
		t.Fatalf("indentJSON() error = %v", err)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("indentJSON() = %q, want indented output", got)
	}
	if !strings.Contains(got, `"supported_parameters"`) {
		t.Errorf("indentJSON() = %q, want original keys preserved", got)
	}
}

func TestRunModelsDetailListsForAllProviders(t *testing.T) {
	cases := []struct {
		name         string
		providerType provider.Type
	}{
		{"openai", provider.OpenAI},
		{"apimart", provider.APIMart},
		{"openlux", provider.OpenLux},
		{"modelscope", provider.ModelScope},
		{"agnes", provider.Agnes},
		{"gemini", provider.Gemini},
		{"bailian", provider.Bailian},
		{"zeekai", provider.Zeekai},
		{"pollinations", provider.Pollinations},
		{"unknown", provider.Unknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var listHit, singleHit bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/models") {
					listHit = true
					w.Write([]byte(`{"data":[{"id":"test/model","object":"model","owned_by":"tester"}]}`))
					return
				}
				singleHit = true
				w.WriteHeader(http.StatusNotFound)
			}))
			defer srv.Close()

			p := &provider.EffectiveProvider{
				BaseURL:      srv.URL,
				Type:         types.ProviderOpenAI,
				ProviderType: tc.providerType,
			}
			if err := runModelsDetail("test/model", p); err != nil {
				t.Fatalf("runModelsDetail() error = %v", err)
			}
			if !listHit {
				t.Errorf("provider %s: list endpoint was never called", tc.name)
			}
			if singleHit {
				t.Errorf("provider %s: GET /models/{id} was probed, want list-only", tc.name)
			}
		})
	}
}

func TestRunModelsModelFlagResolvesDetail(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"mimo-v2.6-flash","object":"model","owned_by":"xiaomi"}]}`))
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{
		BaseURL:      srv.URL,
		APIKey:       "sk-test",
		Type:         types.ProviderOpenAI,
		ProviderType: provider.OpenAI,
	}

	prevModel, prevType, prevPrice, prevDryRun := options.Shared.Model, modelType, priceArg, modelsDryRun
	defer func() {
		options.Shared.Model, modelType, priceArg, modelsDryRun = prevModel, prevType, prevPrice, prevDryRun
	}()
	options.Shared.Model, modelType, priceArg, modelsDryRun = "", "", "", false

	cmd := NewCommand(func() Deps {
		return Deps{
			Model:           options.Shared.Model,
			ResolveProvider: func(string) *provider.EffectiveProvider { return p },
		}
	})
	cmd.Flags().StringVar(&options.Shared.Model, "model", "", "test-only stand-in for the root persistent flag")
	cmd.SetArgs([]string{"--model", "mimo-v2.6-flash"})

	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute() error = %v", err)
		}
	})

	if gotPath != "/v1/models" {
		t.Errorf("request path = %q, want /v1/models", gotPath)
	}
	if !strings.Contains(out, "mimo-v2.6-flash") || !strings.Contains(out, "Owned by: xiaomi") {
		t.Errorf("output = %q, want the model detail", out)
	}
}

func TestModelsDryRunMakesNoRequests(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{
		BaseURL:      srv.URL,
		APIKey:       "sk-test",
		Type:         types.ProviderOpenAI,
		ProviderType: provider.OpenAI,
	}

	prevDryRun, prevD := modelsDryRun, d
	defer func() { modelsDryRun, d = prevDryRun, prevD }()
	modelsDryRun = true
	d = Deps{APIBase: srv.URL}

	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{"list", func() error { return runModelsOpenAI(p) }, "curl -X GET " + srv.URL + "/v1/models"},
		{"detail", func() error { return runModelsDetail("test/model", p) }, "curl -X GET " + srv.URL + "/v1/models"},
		{"discovery", func() error {
			or := &provider.EffectiveProvider{BaseURL: srv.URL + "/api/v1", ProviderType: provider.OpenRouter}
			return runModelsOpenRouterDiscovery(or, "image")
		}, "curl -X GET " + srv.URL + "/api/v1/images/models"},
		{"openrouter detail", func() error {
			or := &provider.EffectiveProvider{BaseURL: srv.URL + "/api/v1", APIKey: "sk-test", ProviderType: provider.OpenRouter}
			return runModelsOpenRouterDetail("openai/gpt-4o", or)
		}, "curl -X GET " + srv.URL + "/api/v1/model/openai/gpt-4o"},
		{"marketplace", func() error { return runModelsMarketplace("image") }, "curl -X GET " + srv.URL + "/api/marketplace/models"},
		{"pricing", func() error { return runModelsPricing("gpt-4o") }, "curl -X GET " + srv.URL + "/api/pricing/model?model=gpt-4o"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := tc.run(); err != nil {
					t.Fatalf("%s dry-run error = %v", tc.name, err)
				}
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s output = %q, want it to contain %q", tc.name, out, tc.want)
			}
		})
	}

	if hits != 0 {
		t.Errorf("dry-run made %d HTTP request(s), want 0", hits)
	}
}

// resetModelsFlags isolates tests that touch the package-level flag variables.
func resetModelsFlags(t *testing.T) {
	t.Helper()
	prevModelType, prevPrice, prevDryRun, prevList, prevD := modelType, priceArg, modelsDryRun, modelsListProviders, d
	t.Cleanup(func() {
		modelType, priceArg, modelsDryRun, modelsListProviders, d = prevModelType, prevPrice, prevDryRun, prevList, prevD
	})
	modelType, priceArg, modelsDryRun, modelsListProviders = "", "", false, false
}

func TestHostOf(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "-"},
		{"https://api.example.com/v1", "api.example.com"},
		{"api.example.com", "api.example.com"},
		{"api.example.com/v1", "api.example.com"},
		{"https://user:pass@host.example:8080/v1?x=1", "host.example"},
		{"://bad", "-"},
	}
	for _, tc := range tests {
		if got := hostOf(tc.in); got != tc.want {
			t.Errorf("hostOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRunModelsListProviders(t *testing.T) {
	resetModelsFlags(t)
	d = Deps{
		APIBase: "https://api.apimart.ai",
		Providers: map[string]*types.NamedProvider{
			"zeta":   {APIKey: "secret-zeta", BaseURL: "https://api.zeta.example/v1"},
			"alpha":  {APIKey: "secret-alpha", BaseURL: "https://alpha.example"},
			"nokey":  {BaseURL: "https://nokey.example"},
			"loop":   {BaseURL: "http://127.0.0.1:11434/v1"},
			"ollama": {Type: types.ProviderOllama, BaseURL: "http://localhost:11434/v1"},
			"nil":    nil,
		},
		ResolveProvider: func(string) *provider.EffectiveProvider {
			t.Fatal("--list-providers must not resolve or call a provider")
			return nil
		},
	}

	cmd := NewCommand(func() Deps { return d })
	cmd.SetArgs([]string{"--list-providers"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute() error = %v", err)
		}
	})

	for _, want := range []string{
		"Configured providers (5)", "NAME", "TYPE", "HOST", "KEY",
		"alpha", "api.zeta.example", "nokey", "nokey.example", "loop",
		"ollama", "n/a", "yes", "no", "aigc-cli models --provider <name>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	}
	if strings.Index(out, "alpha") > strings.Index(out, "zeta") {
		t.Errorf("output = %q, want providers sorted alphabetically", out)
	}
	for _, leak := range []string{"secret-alpha", "secret-zeta"} {
		if strings.Contains(out, leak) {
			t.Errorf("output = %q, must never print API key values", out)
		}
	}
}

func TestRunModelsListProvidersEmpty(t *testing.T) {
	resetModelsFlags(t)
	d = Deps{ResolveProvider: func(string) *provider.EffectiveProvider { return nil }}

	cmd := NewCommand(func() Deps { return d })
	cmd.SetArgs([]string{"--list-providers"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute() error = %v", err)
		}
	})
	if !strings.Contains(out, "No providers configured") {
		t.Errorf("output = %q, want the empty-provider hint", out)
	}
}

func TestRunModelsNoKeyListsCandidates(t *testing.T) {
	resetModelsFlags(t)
	p := &provider.EffectiveProvider{BaseURL: "https://api.nokey.example/v1", Type: types.ProviderOpenAI}
	d = Deps{
		APIBase: "https://api.apimart.ai",
		Providers: map[string]*types.NamedProvider{
			"apimart": {APIKey: "secret-apimart", BaseURL: "https://api.apimart.ai"},
			"keyless": {BaseURL: "https://keyless.example"},
		},
		ResolveProvider: func(string) *provider.EffectiveProvider { return p },
	}

	cmd := NewCommand(func() Deps { return d })
	cmd.SetArgs([]string{})
	var execErr error
	out := captureStdout(t, func() { execErr = cmd.Execute() })

	if execErr == nil || !strings.Contains(execErr.Error(), "no API key") {
		t.Fatalf("execute() error = %v, want a missing-key error", execErr)
	}
	for _, want := range []string{
		"No API key for models", "Candidate providers (2)",
		"apimart", "api.apimart.ai", "keyless", "keyless.example",
		"Pick one with: aigc-cli models --provider <name>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	}
	if strings.Contains(out, "secret-apimart") {
		t.Errorf("output = %q, must never print API key values", out)
	}
}

func TestRunModelsNoKeyNoProviders(t *testing.T) {
	resetModelsFlags(t)
	p := &provider.EffectiveProvider{BaseURL: "https://api.nokey.example/v1", Type: types.ProviderOpenAI}
	d = Deps{ResolveProvider: func(string) *provider.EffectiveProvider { return p }}

	cmd := NewCommand(func() Deps { return d })
	cmd.SetArgs([]string{})
	var execErr error
	out := captureStdout(t, func() { execErr = cmd.Execute() })

	if execErr == nil || !strings.Contains(execErr.Error(), "no API key") {
		t.Fatalf("execute() error = %v, want a missing-key error", execErr)
	}
	if !strings.Contains(out, "No providers configured") {
		t.Errorf("output = %q, want the no-provider hint", out)
	}
}

func TestRunModelsDetailNotFoundInList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Write([]byte(`{"data":[{"id":"other/model"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{BaseURL: srv.URL, Type: types.ProviderOpenAI, ProviderType: provider.OpenAI}
	err := runModelsDetail("missing/model", p)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("runModelsDetail() error = %v, want not-found after listing", err)
	}
}
