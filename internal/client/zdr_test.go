package client

import (
	"encoding/json"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const testOpenRouterURL = "https://openrouter.ai/api/v1"

func TestApplyOpenRouterZDR(t *testing.T) {
	cases := []struct {
		name          string
		body          string
		zdr           bool
		baseURL       string
		wantProvider  bool
		wantOrderKept string
	}{
		{name: "off leaves body untouched", body: `{"model":"m"}`, zdr: false, baseURL: testOpenRouterURL},
		{name: "non-openrouter is a no-op", body: `{"model":"m"}`, zdr: true, baseURL: "https://api.openai.com/v1"},
		{name: "openrouter injects routing object", body: `{"model":"m"}`, zdr: true, baseURL: testOpenRouterURL, wantProvider: true},
		{name: "preserves existing provider fields", body: `{"model":"m","provider":{"order":["a"],"allow_fallbacks":false}}`, zdr: true, baseURL: testOpenRouterURL, wantProvider: true, wantOrderKept: "a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyOpenRouterZDR([]byte(tc.body), tc.zdr, tc.baseURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(got, &obj); err != nil {
				t.Fatalf("result is not a JSON object: %v (body=%s)", err, got)
			}
			rawProvider, hasProvider := obj["provider"]
			if hasProvider != tc.wantProvider {
				t.Fatalf("provider present = %v, want %v (body=%s)", hasProvider, tc.wantProvider, got)
			}
			if !hasProvider {
				return
			}
			var prefs map[string]any
			if err := json.Unmarshal(rawProvider, &prefs); err != nil {
				t.Fatalf("provider is not an object: %v", err)
			}
			if prefs["zdr"] != true {
				t.Errorf("zdr = %v, want true", prefs["zdr"])
			}
			if prefs["data_collection"] != "deny" {
				t.Errorf("data_collection = %v, want deny", prefs["data_collection"])
			}
			if tc.wantOrderKept != "" {
				order, _ := prefs["order"].([]any)
				if len(order) != 1 || order[0] != tc.wantOrderKept {
					t.Errorf("provider.order not preserved: %v", prefs["order"])
				}
			}
		})
	}
}

func TestApplyOpenRouterZDR_nonObjectUnchanged(t *testing.T) {
	for _, body := range []string{`[1,2,3]`, `"hello"`, `42`, ``} {
		got, err := ApplyOpenRouterZDR([]byte(body), true, testOpenRouterURL)
		if err != nil {
			t.Fatalf("body %q: unexpected error: %v", body, err)
		}
		if string(got) != body {
			t.Errorf("body %q changed to %q", body, got)
		}
	}
}

func TestChatBody_appliesZDRForOpenRouterOnly(t *testing.T) {
	req := &types.ChatRequest{
		Model:    "openai/gpt-4o-mini",
		Messages: []types.ChatMessage{{Role: "user", Content: "hi"}},
	}

	on, err := ChatBody(req, true, testOpenRouterURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(on, &obj); err != nil {
		t.Fatalf("not an object: %v", err)
	}
	if _, ok := obj["provider"]; !ok {
		t.Errorf("provider missing when zdr on: %s", on)
	}

	off, err := ChatBody(req, false, testOpenRouterURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var offObj map[string]json.RawMessage
	if err := json.Unmarshal(off, &offObj); err != nil {
		t.Fatalf("not an object: %v", err)
	}
	if _, ok := offObj["provider"]; ok {
		t.Errorf("provider present when zdr off: %s", off)
	}
}

func TestChatBody_patchesVerbatimJSON(t *testing.T) {
	req := &types.ChatRequest{
		RawJSON: json.RawMessage(`{"model":"m","messages":[{"role":"user","content":"hi"}],"provider":{"order":["a"]}}`),
	}
	got, err := ChatBody(req, true, testOpenRouterURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatalf("not an object: %v", err)
	}
	var prefs map[string]any
	if err := json.Unmarshal(obj["provider"], &prefs); err != nil {
		t.Fatalf("provider not an object: %v", err)
	}
	if prefs["zdr"] != true || prefs["data_collection"] != "deny" {
		t.Errorf("verbatim body not patched: %s", got)
	}
	if order, _ := prefs["order"].([]any); len(order) != 1 || order[0] != "a" {
		t.Errorf("verbatim provider fields not preserved: %s", got)
	}
}

func TestNewFromProvider_propagatesZDR(t *testing.T) {
	p := &provider.EffectiveProvider{Type: types.ProviderOpenAI, BaseURL: testOpenRouterURL, ZDR: true}
	if c := NewFromProvider(p); !c.zdr {
		t.Errorf("NewFromProvider did not propagate ZDR")
	}
	p.ZDR = false
	if c := NewFromProvider(p); c.zdr {
		t.Errorf("NewFromProvider propagated ZDR when disabled")
	}
}
