package service

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	fakeGlobalKey = "sk-global-0123456789abcdefghij"
	fakeNamedKey  = "sk-named-abcdefghijklmnopqrst"
	fakeSearchKey = "fc-search-abcdefghijklmnopqrst"
	fakeURLKey    = "sk-urlkey-abcdefghijklmnop"
	fakeProxyPass = "proxyS3cretValue"
)

func TestMaskConfigSecretsMasksEverySecret(t *testing.T) {
	cfg := &types.Config{
		APIKey:    fakeGlobalKey,
		BaseURL:   "https://api.example.com/v1?key=" + fakeURLKey,
		HTTPProxy: "http://user:" + fakeProxyPass + "@127.0.0.1:7897",
		Providers: map[string]*types.NamedProvider{
			"relay": {APIKey: fakeNamedKey, BaseURL: "https://relay.example.com/v1"},
		},
		WebSearch: map[string]*types.WebSearchProvider{
			"doubao": {Type: "doubao", APIKey: fakeSearchKey},
		},
	}

	masked := MaskConfigSecrets(cfg)

	if cfg.APIKey != fakeGlobalKey ||
		cfg.Providers["relay"].APIKey != fakeNamedKey ||
		cfg.WebSearch["doubao"].APIKey != fakeSearchKey {
		t.Fatal("MaskConfigSecrets mutated its input")
	}
	if masked.APIKey != MaskKey(fakeGlobalKey) {
		t.Errorf("global key = %q, want %q", masked.APIKey, MaskKey(fakeGlobalKey))
	}
	if got := masked.Providers["relay"].APIKey; got != MaskKey(fakeNamedKey) {
		t.Errorf("provider key = %q, want %q", got, MaskKey(fakeNamedKey))
	}
	if got := masked.WebSearch["doubao"].APIKey; got != MaskKey(fakeSearchKey) {
		t.Errorf("web_search key = %q, want %q", got, MaskKey(fakeSearchKey))
	}
	if !strings.Contains(masked.BaseURL, "key="+urlMask) {
		t.Errorf("base_url query secret not redacted: %q", masked.BaseURL)
	}
	if !strings.Contains(masked.HTTPProxy, ":"+urlMask+"@") {
		t.Errorf("proxy password not redacted: %q", masked.HTTPProxy)
	}

	b, err := yaml.Marshal(masked)
	if err != nil {
		t.Fatalf("marshal masked config: %v", err)
	}
	out := string(b)
	for _, secret := range []string{fakeGlobalKey, fakeNamedKey, fakeSearchKey, fakeURLKey, fakeProxyPass} {
		if strings.Contains(out, secret) {
			t.Errorf("full secret leaked into YAML output")
		}
		if strings.Contains(out, secret[:8]) {
			t.Errorf("secret prefix leaked into YAML output")
		}
	}
	if !strings.Contains(out, MaskKey(fakeGlobalKey)) {
		t.Error("masked global key missing from YAML output")
	}
}

func TestMaskConfigSecretsHandlesNil(t *testing.T) {
	if got := MaskConfigSecrets(nil); got != nil {
		t.Fatalf("MaskConfigSecrets(nil) = %v, want nil", got)
	}

	out := MaskConfigSecrets(&types.Config{})
	if out == nil {
		t.Fatal("MaskConfigSecrets returned nil for empty config")
	}
	if out.Providers != nil || out.WebSearch != nil {
		t.Fatal("nil maps must stay nil")
	}

	withNilEntry := &types.Config{Providers: map[string]*types.NamedProvider{"empty": nil}}
	if got := MaskConfigSecrets(withNilEntry).Providers["empty"]; got != nil {
		t.Fatalf("nil provider entry = %v, want nil", got)
	}
}

func TestMaskConfigSecretsKeepsUnsetKeysOmitted(t *testing.T) {
	cfg := &types.Config{
		Providers: map[string]*types.NamedProvider{
			"ollama": {Type: types.ProviderOllama, BaseURL: "http://localhost:11434/v1"},
		},
	}

	masked := MaskConfigSecrets(cfg)
	if got := masked.Providers["ollama"].APIKey; got != "" {
		t.Fatalf("unset provider key = %q, want empty", got)
	}

	b, err := yaml.Marshal(masked)
	if err != nil {
		t.Fatalf("marshal masked config: %v", err)
	}
	if strings.Contains(string(b), "***") {
		t.Error("unset api_key must stay omitted from YAML output")
	}
}
