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

// captureStderr runs fn and returns everything written to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stderr = old }()
	fn()
	w.Close()
	os.Stderr = old
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
						"seed": {"type": "boolean"},
						"aspect_ratio": {"type": "enum", "values": ["1:1", "4:3", "3:4", "16:9", "9:16", "2:3", "3:2", "21:9"]}
					},
					"supports_streaming": false
				}
			]
		}`))
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{BaseURL: srv.URL, ProviderType: provider.OpenRouter}
	out := captureStdout(t, func() {
		if err := runModelsOpenRouterDiscovery(p, "image"); err != nil {
			t.Fatalf("runModelsOpenRouterDiscovery() error = %v", err)
		}
	})
	// Enum values must not be truncated: this list is the only place image
	// sizing capabilities are shown.
	if !strings.Contains(out, "aspect_ratio=1:1|4:3|3:4|16:9|9:16|2:3|3:2|21:9") {
		t.Errorf("output should print every enum value, got:\n%s", out)
	}
	// Parameters must be sorted so repeated runs are stable.
	if !strings.Contains(out, "aspect_ratio=") ||
		strings.Index(out, "aspect_ratio=") > strings.Index(out, "n=1-4") ||
		strings.Index(out, "n=1-4") > strings.Index(out, "seed") {
		t.Errorf("output should list parameters in a stable sorted order, got:\n%s", out)
	}
}

func TestNormalizeOpenRouterModality(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "text", false},
		{"chat", "text", false},
		{"text", "text", false},
		{"image", "image", false},
		{"video", "video", false},
		{"audio", "audio", false},
		{"music", "audio", false},
		{"speech", "speech", false},
		{"tts", "speech", false},
		{"transcription", "transcription", false},
		{"stt", "transcription", false},
		{"asr", "transcription", false},
		{"embeddings", "embeddings", false},
		{"embedding", "embeddings", false},
		{"rerank", "rerank", false},
		{"decisions", "decisions", false},
		{"decision", "decisions", false},
		{"all", "all", false},
		{" SPEECH ", "speech", false},
		{"bogus", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := normalizeOpenRouterModality(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeOpenRouterModality(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeOpenRouterModality(%q) error = %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("normalizeOpenRouterModality(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRunModelsOpenRouterStandardDiscovery(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": [
				{
					"id": "heygen/heygen-voice-1-turbo:free",
					"name": "HeyGen: HeyGen Voice Turbo (free)",
					"architecture": {"input_modalities": ["text"], "output_modalities": ["speech"]},
					"supported_voices": ["nadine", "bram"],
					"pricing": {"prompt": "0", "completion": "0", "overrides": [1, 2]},
					"expiration_date": "2026-11-30"
				},
				{
					"id": "acme/voice",
					"name": "Acme Voice",
					"architecture": {"input_modalities": ["text"], "output_modalities": ["speech"]},
					"pricing": {"prompt": "0.000004", "completion": "0"}
				}
			]
		}`))
	}))
	defer srv.Close()

	prevPrice := cmdPriceChanged
	cmdPriceChanged = true
	defer func() { cmdPriceChanged = prevPrice }()

	p := &provider.EffectiveProvider{BaseURL: srv.URL + "/api/v1", ProviderType: provider.OpenRouter}
	var err error
	out := captureStdout(t, func() { err = runModelsOpenRouterDiscovery(p, "tts") })
	if err != nil {
		t.Fatalf("runModelsOpenRouterDiscovery() error = %v", err)
	}

	if gotPath != "/api/v1/models" {
		t.Errorf("request path = %q, want /api/v1/models", gotPath)
	}
	if gotQuery != "output_modalities=speech" {
		t.Errorf("query = %q, want output_modalities=speech", gotQuery)
	}
	for _, want := range []string{
		"Text-to-Speech Models (2)",
		"heygen/heygen-voice-1-turbo:free  (free)",
		"voices: nadine, bram",
		"$4/M in",
		"expires 2026-11-30",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output = %q, want it to contain %q", out, want)
		}
	}
}

func TestRunModelsOpenRouterTextIsCompact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"acme/chat:free","name":"Acme Chat","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"pricing":{"prompt":"0","completion":"0"}}]}`))
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{BaseURL: srv.URL + "/api/v1", ProviderType: provider.OpenRouter}
	var err error
	out := captureStdout(t, func() { err = runModelsOpenRouterDiscovery(p, "chat") })
	if err != nil {
		t.Fatalf("runModelsOpenRouterDiscovery() error = %v", err)
	}
	if !strings.Contains(out, "Text / Chat Models (1)") {
		t.Errorf("output = %q, want the text/chat header", out)
	}
	if !strings.Contains(out, "acme/chat:free  (free)") {
		t.Errorf("output = %q, want the compact free marker", out)
	}
	if strings.Contains(out, "─ Acme Chat") {
		t.Errorf("output = %q, want one line per model for text listings", out)
	}
}

func TestRunModelsOpenRouterUnknownType(t *testing.T) {
	p := &provider.EffectiveProvider{BaseURL: "https://openrouter.ai/api/v1", ProviderType: provider.OpenRouter}
	err := runModelsOpenRouterDiscovery(p, "bogus")
	if err == nil || !strings.Contains(err.Error(), "unknown model type") {
		t.Fatalf("runModelsOpenRouterDiscovery() error = %v, want unknown-type error", err)
	}
}

func TestRunModelsMarketplaceUnknownType(t *testing.T) {
	err := runModelsMarketplace("speech")
	if err == nil || !strings.Contains(err.Error(), "unknown model type") {
		t.Fatalf("runModelsMarketplace() error = %v, want unknown-type error", err)
	}
}

func TestModelsBarePriceFlag(t *testing.T) {
	resetModelsFlags(t)

	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"acme/chat","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"pricing":{"prompt":"0.000002","completion":"0.000004"}}]}`))
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{
		BaseURL:      srv.URL + "/api/v1",
		APIKey:       "sk-test",
		ProviderType: provider.OpenRouter,
	}
	d = Deps{ResolveProvider: func(string) *provider.EffectiveProvider { return p }}

	cmd := NewCommand(func() Deps { return d })
	cmd.SetArgs([]string{"--price"})
	var err error
	out := captureStdout(t, func() { err = cmd.Execute() })
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if gotPath != "/api/v1/models" || gotQuery != "output_modalities=text" {
		t.Errorf("request = %q?%q, want the standard text listing", gotPath, gotQuery)
	}
	if !strings.Contains(out, "$2/M in · $4/M out") {
		t.Errorf("output = %q, want the pricing column", out)
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

	prevDryRun, prevD, prevPrice := modelsDryRun, d, cmdPriceChanged
	defer func() { modelsDryRun, d, cmdPriceChanged = prevDryRun, prevD, prevPrice }()
	modelsDryRun = true
	cmdPriceChanged = false
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
		{"discovery speech", func() error {
			or := &provider.EffectiveProvider{BaseURL: srv.URL + "/api/v1", ProviderType: provider.OpenRouter}
			return runModelsOpenRouterDiscovery(or, "speech")
		}, "curl -X GET " + srv.URL + "/api/v1/models?output_modalities=speech"},
		{"discovery all", func() error {
			or := &provider.EffectiveProvider{BaseURL: srv.URL + "/api/v1", ProviderType: provider.OpenRouter}
			return runModelsOpenRouterDiscovery(or, "all")
		}, "curl -X GET " + srv.URL + "/api/v1/models?output_modalities=all"},
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
	prevModelType, prevPrice, prevDryRun, prevList := modelType, priceArg, modelsDryRun, modelsListProviders
	prevD, prevPriceChanged := d, cmdPriceChanged
	t.Cleanup(func() {
		modelType, priceArg, modelsDryRun, modelsListProviders = prevModelType, prevPrice, prevDryRun, prevList
		d, cmdPriceChanged = prevD, prevPriceChanged
	})
	modelType, priceArg, modelsDryRun, modelsListProviders = "", "", false, false
	cmdPriceChanged = false
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

func TestRunModelsOpenRouterImagePriceUsesStandardEndpoint(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"acme/image","architecture":{"input_modalities":["text"],"output_modalities":["image"]},"pricing":{"image":"0.04"}}]}`))
	}))
	defer srv.Close()

	prevPrice := cmdPriceChanged
	cmdPriceChanged = true
	defer func() { cmdPriceChanged = prevPrice }()

	p := &provider.EffectiveProvider{BaseURL: srv.URL + "/api/v1", ProviderType: provider.OpenRouter}
	var err error
	out := captureStdout(t, func() { err = runModelsOpenRouterDiscovery(p, "image") })
	if err != nil {
		t.Fatalf("runModelsOpenRouterDiscovery() error = %v", err)
	}
	if gotPath != "/api/v1/models" {
		t.Errorf("path = %q, want the standard /api/v1/models endpoint when pricing is shown", gotPath)
	}
	if gotQuery != "output_modalities=image" {
		t.Errorf("query = %q, want output_modalities=image", gotQuery)
	}
	if !strings.Contains(out, "$0.04/image") {
		t.Errorf("output = %q, want per-image pricing", out)
	}
}

func TestModelsPriceModelRoutesToOpenRouterDetail(t *testing.T) {
	resetModelsFlags(t)

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"id":"acme/chat","pricing":{"prompt":"0.000002"}}}`))
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{
		BaseURL:      srv.URL + "/api/v1",
		APIKey:       "sk-test",
		ProviderType: provider.OpenRouter,
	}
	d = Deps{ResolveProvider: func(string) *provider.EffectiveProvider { return p }}

	cmd := NewCommand(func() Deps { return d })
	cmd.SetArgs([]string{"--price", "acme/chat"})
	var err error
	out := captureStdout(t, func() { err = cmd.Execute() })
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if gotPath != "/api/v1/model/acme/chat" {
		t.Errorf("path = %q, want the singular model endpoint", gotPath)
	}
	if !strings.Contains(out, `"pricing"`) {
		t.Errorf("output = %q, want the model record", out)
	}
}

func TestOpenRouterModelDetailShowsImageCapabilities(t *testing.T) {
	imageCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/model/qwen/qwen-image-2.1-turbo":
			_, _ = w.Write([]byte(`{"data":{"id":"qwen/qwen-image-2.1-turbo","architecture":{"output_modalities":["image"]},"supported_parameters":["max_tokens"]}}`))
		case "/v1/model/openai/gpt-4o":
			_, _ = w.Write([]byte(`{"data":{"id":"openai/gpt-4o","architecture":{"output_modalities":["text"]},"supported_parameters":["tools"]}}`))
		case "/v1/images/models":
			imageCalls++
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen/qwen-image-2.1-turbo","supported_parameters":{
				"aspect_ratio":{"type":"enum","values":["1:1","16:9"]},
				"resolution":{"type":"enum","values":["1K","2K"]},
				"n":{"type":"range","min":1,"max":4},
				"seed":{"type":"boolean"}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := &provider.EffectiveProvider{BaseURL: srv.URL + "/v1", ProviderType: provider.OpenRouter, APIKey: "k"}

	var stdout, stderr string
	stderr = captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			if err := runModelsOpenRouterDetail("qwen/qwen-image-2.1-turbo", p); err != nil {
				t.Fatalf("runModelsOpenRouterDetail() error = %v", err)
			}
		})
	})

	if !strings.Contains(stdout, `"id": "qwen/qwen-image-2.1-turbo"`) {
		t.Errorf("stdout should keep the verbatim JSON model record, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "Image capabilities") {
		t.Errorf("capability block must not break stdout JSON, got:\n%s", stdout)
	}
	for _, want := range []string{
		"Image capabilities (from /v1/images/models):",
		"aspect_ratio=1:1|16:9",
		"resolution=1K|2K",
		"n=1-4",
		"--size: 16:9 (ratio), 2K (tier), 2K@16:9 (both)",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}

	stderr = captureStderr(t, func() {
		captureStdout(t, func() {
			if err := runModelsOpenRouterDetail("openai/gpt-4o", p); err != nil {
				t.Errorf("runModelsOpenRouterDetail() error = %v", err)
			}
		})
	})
	if imageCalls != 1 {
		t.Errorf("/images/models calls = %d, want 1 (text models must not trigger the lookup)", imageCalls)
	}
	if stderr != "" {
		t.Errorf("text model should print no capability block, got:\n%s", stderr)
	}
}

func TestMatchesLike(t *testing.T) {
	orig := modelLike
	defer func() { modelLike = orig }()

	modelLike = ""
	if !matchesLike("anything") {
		t.Error("empty --like should match everything")
	}

	modelLike = "RECRAFT"
	if !matchesLike("recraft/recraft-v4.1-flash", "") {
		t.Error("match should be case-insensitive on id")
	}
	if !matchesLike("x/y", "Recraft V4") {
		t.Error("match should look at the display name")
	}
	if matchesLike("openai/gpt-image-2", "GPT Image 2") {
		t.Error("non-matching model should be filtered out")
	}
}

func TestOpenRouterMediaDiscoveryLikeFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"recraft/recraft-v4","name":"Recraft V4","supported_parameters":{}},
			{"id":"openai/gpt-image-2","name":"GPT Image 2","supported_parameters":{}}
		]}`))
	}))
	defer srv.Close()

	orig := modelLike
	defer func() { modelLike = orig }()

	modelLike = "recraft"
	out := captureStdout(t, func() {
		if err := runOpenRouterMediaDiscovery(srv.URL, "image"); err != nil {
			t.Fatalf("runOpenRouterMediaDiscovery() error = %v", err)
		}
	})
	if !strings.Contains(out, "recraft/recraft-v4") || strings.Contains(out, "gpt-image-2") {
		t.Errorf("filter should keep only matching models, got:\n%s", out)
	}
	if !strings.Contains(out, "Image Models (1)") {
		t.Errorf("count should reflect the filtered list, got:\n%s", out)
	}

	modelLike = "no-such-model"
	out = captureStdout(t, func() {
		if err := runOpenRouterMediaDiscovery(srv.URL, "image"); err != nil {
			t.Fatalf("runOpenRouterMediaDiscovery() error = %v", err)
		}
	})
	if !strings.Contains(out, `No models match --like "no-such-model"`) {
		t.Errorf("empty filter result should explain itself, got:\n%s", out)
	}
}

func TestPrintOpenAIModelDetailShowsExtraFields(t *testing.T) {
	model := &types.OpenAIModel{
		ID:                     "agnes-image-2.5-flash",
		Object:                 "model",
		Created:                1626777600,
		OwnedBy:                "custom",
		SupportedEndpointTypes: []string{"openai"},
	}
	out := captureStdout(t, func() { printOpenAIModelDetail(model) })
	for _, want := range []string{"agnes-image-2.5-flash", "Object:   model", "Owned by: custom", "Endpoints: openai"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail missing %q:\n%s", want, out)
		}
	}
}
