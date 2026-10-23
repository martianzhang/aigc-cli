package search

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNewSFXMintProviderDefaults(t *testing.T) {
	p := NewSFXMintProvider()
	if p.Name() != "sfxmint" {
		t.Errorf("Name() = %q, want sfxmint", p.Name())
	}
	if p.baseURL != defaultSFXMintBaseURL {
		t.Errorf("baseURL = %q, want %q", p.baseURL, defaultSFXMintBaseURL)
	}
	if p.baseURL != "https://sfxmint.com" {
		t.Errorf("default base URL = %q, want https://sfxmint.com", p.baseURL)
	}
}

func TestSFXMintProviderImplementsCapabilities(t *testing.T) {
	p := NewSFXMintProvider()
	if _, ok := any(p).(Provider); !ok {
		t.Error("SFXMintProvider must implement Provider")
	}
	if _, ok := any(p).(RawProvider); !ok {
		t.Error("SFXMintProvider must implement RawProvider")
	}
	if _, ok := any(p).(MediaProvider); !ok {
		t.Error("Provider must implement MediaProvider")
	}
	if got := strings.Join(p.MediaTypes(), ","); got != "sfx" {
		t.Errorf("MediaTypes() = %q, want sfx", got)
	}
	if _, ok := any(p).(Previewer); !ok {
		t.Error("SFXMintProvider must implement Previewer")
	}
}

func TestSFXMintSearchArrayResponse(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		io.WriteString(w, `[
			{"slug":"metal-door-slam-02","title":"Metal Door Slam 02",
			 "page_url":"https://sfxmint.com/sounds/metal-door-slam-02",
			 "mp3_url":"https://sfxmint.com/dl/metal-door-slam-02.mp3",
			 "wav_url":"https://sfxmint.com/dl/metal-door-slam-02.wav"},
			{"slug":"door-close-43","title":"","page_url":"https://sfxmint.com/sounds/door-close-43",
			 "mp3_url":"https://sfxmint.com/dl/door-close-43.mp3","wav_url":""}
		]`)
	}))
	defer srv.Close()

	p := &SFXMintProvider{baseURL: srv.URL}
	results, raw, err := p.SearchRaw("door slam", 5, Options{})
	if err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}

	if gotPath != "/api/v1/search" {
		t.Errorf("path = %q, want /api/v1/search", gotPath)
	}
	if got := gotQuery.Get("q"); got != "door slam" {
		t.Errorf("q = %q, want door slam", got)
	}
	if got := gotQuery.Get("limit"); got != "5" {
		t.Errorf("limit = %q, want 5", got)
	}

	want := []Result{
		{URL: "https://sfxmint.com/dl/metal-door-slam-02.wav", Title: "Metal Door Slam 02"},
		{URL: "https://sfxmint.com/dl/door-close-43.mp3", Title: "door-close-43"},
	}
	if len(results) != len(want) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(want))
	}
	for i := range want {
		if results[i] != want[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], want[i])
		}
	}
	if !strings.Contains(string(raw), `"wav_url"`) {
		t.Errorf("raw response was not returned verbatim: %s", raw)
	}
}

func TestSFXMintSearchStructuredResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"candidates":[
			{"slug":"ui-click-30","title":"User Interface Click 30",
			 "wav_url":"https://sfxmint.com/dl/ui-click-30.wav"}
		]}`)
	}))
	defer srv.Close()

	p := &SFXMintProvider{baseURL: srv.URL}
	results, _, err := p.SearchRaw("click", 3, Options{Extra: json.RawMessage(`{"response":"structured"}`)})
	if err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}
	if len(results) != 1 || results[0].Title != "User Interface Click 30" {
		t.Fatalf("results = %+v, want the structured candidate", results)
	}
}

func TestSFXMintInjectKeepsReservedParams(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		io.WriteString(w, `[]`)
	}))
	defer srv.Close()

	p := &SFXMintProvider{baseURL: srv.URL}
	opts := Options{Extra: json.RawMessage(`{"max_duration_ms":2000,"loop":true,"q":"hacked","limit":99}`)}
	if _, _, err := p.SearchRaw("rain window", 5, opts); err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}

	if got := gotQuery.Get("max_duration_ms"); got != "2000" {
		t.Errorf("max_duration_ms = %q, want 2000", got)
	}
	if got := gotQuery.Get("loop"); got != "true" {
		t.Errorf("loop = %q, want true", got)
	}
	if got := gotQuery.Get("q"); got != "rain window" {
		t.Errorf("q = %q, want the CLI-owned query", got)
	}
	if got := gotQuery.Get("limit"); got != "5" {
		t.Errorf("limit = %q, want the CLI-owned limit", got)
	}
}

func TestSFXMintErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	p := &SFXMintProvider{baseURL: srv.URL}
	if _, _, err := p.SearchRaw("q", 5, Options{}); err == nil {
		t.Fatal("SearchRaw() error = nil, want API error")
	}
}

func TestSFXMintPreview(t *testing.T) {
	p := NewSFXMintProvider()
	plan, err := p.Preview("door slam", 5, Options{
		Extra: json.RawMessage(`{"max_duration_ms":2000}`),
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if plan.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", plan.Method)
	}
	u, err := url.Parse(plan.URL)
	if err != nil {
		t.Fatalf("parse preview URL: %v", err)
	}
	values := u.Query()
	if got := values.Get("q"); got != "door slam" {
		t.Errorf("q = %q, want door slam", got)
	}
	if got := values.Get("max_duration_ms"); got != "2000" {
		t.Errorf("max_duration_ms = %q, want 2000", got)
	}
}
