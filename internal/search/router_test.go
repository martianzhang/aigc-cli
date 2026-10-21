package search

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type mockProvider struct {
	name    string
	results []Result
	err     error
	calls   int
	lastQ   string
	lastLim int
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Search(query string, limit int) ([]Result, error) {
	m.calls++
	m.lastQ = query
	m.lastLim = limit
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

type mockRawProvider struct {
	*mockProvider
	raw      json.RawMessage
	lastOpts Options
}

func (m *mockRawProvider) SearchRaw(query string, limit int, opts Options) ([]Result, json.RawMessage, error) {
	m.lastOpts = opts
	results, err := m.Search(query, limit)
	return results, m.raw, err
}

func TestRouterRegisterAndNames(t *testing.T) {
	r := NewRouter()
	r.Register("beta", &mockProvider{name: "beta"})
	r.Register("alpha", &mockProvider{name: "alpha"})

	if _, ok := r.GetProvider("alpha"); !ok {
		t.Error("alpha not registered")
	}
	if _, ok := r.GetProvider("missing"); ok {
		t.Error("missing provider reported as registered")
	}
	got := r.Names()
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("Names() = %v, want [alpha beta]", got)
	}
}

func TestRouterSearchExplicitProvider(t *testing.T) {
	p := &mockProvider{name: "alpha", results: []Result{{URL: "https://a.example"}}}
	r := NewRouter()
	r.Register("alpha", p)

	got, err := r.Search("query", 7, "alpha", Options{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got.Provider != "alpha" || len(got.Results) != 1 {
		t.Errorf("result = %+v, want alpha with one result", got)
	}
	if p.lastQ != "query" || p.lastLim != 7 {
		t.Errorf("provider got query=%q limit=%d", p.lastQ, p.lastLim)
	}
}

func TestRouterSearchUnknownProvider(t *testing.T) {
	r := NewRouter()
	r.Register("alpha", &mockProvider{name: "alpha"})

	_, err := r.Search("q", 5, "missing", Options{})
	if err == nil || !strings.Contains(err.Error(), `"missing"`) || !strings.Contains(err.Error(), "alpha") {
		t.Fatalf("error = %v, want unknown-provider message with available names", err)
	}
}

func TestRouterSearchNoFallback(t *testing.T) {
	bad := &mockProvider{name: "bad", err: errors.New("boom")}
	good := &mockProvider{name: "good", results: []Result{{URL: "https://good.example"}}}
	r := NewRouter()
	r.Register("bad", bad)
	r.Register("good", good)

	_, err := r.Search("q", 5, "bad", Options{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want the selected provider's error", err)
	}
	if good.calls != 0 {
		t.Error("Search fell back to another provider; only the selected one may run")
	}
}

func TestRouterSearchRawAndOptions(t *testing.T) {
	raw := json.RawMessage(`{"ok":true}`)
	p := &mockRawProvider{
		mockProvider: &mockProvider{name: "raw", results: []Result{{URL: "https://raw.example"}}},
		raw:          raw,
	}
	r := NewRouter()
	r.Register("raw", p)

	inject := json.RawMessage(`{"type":"neural"}`)
	got, err := r.Search("q", 5, "raw", Options{Extra: inject})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if string(got.Raw) != string(raw) {
		t.Errorf("Raw = %s, want %s", got.Raw, raw)
	}
	if string(p.lastOpts.Extra) != string(inject) {
		t.Errorf("inject = %s, want %s", p.lastOpts.Extra, inject)
	}
}

func TestRouterSearchInjectUnsupported(t *testing.T) {
	r := NewRouter()
	r.Register("plain", &mockProvider{name: "plain"})

	_, err := r.Search("q", 5, "plain", Options{Extra: json.RawMessage(`{"a":1}`)})
	if err == nil || !strings.Contains(err.Error(), "--inject-json") {
		t.Fatalf("error = %v, want unsupported inject message", err)
	}
}
