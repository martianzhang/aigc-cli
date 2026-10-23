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

func TestNewFreesoundProviderDefaults(t *testing.T) {
	p := NewFreesoundProvider("fs-key")
	if p.Name() != "freesound" {
		t.Errorf("Name() = %q, want freesound", p.Name())
	}
	if p.baseURL != defaultFreesoundBaseURL {
		t.Errorf("baseURL = %q, want %q", p.baseURL, defaultFreesoundBaseURL)
	}
	if p.baseURL != "https://freesound.org" {
		t.Errorf("default base URL = %q, want https://freesound.org", p.baseURL)
	}
}

func TestFreesoundProviderImplementsCapabilities(t *testing.T) {
	p := NewFreesoundProvider("fs-key")
	if _, ok := any(p).(Provider); !ok {
		t.Error("FreesoundProvider must implement Provider")
	}
	if _, ok := any(p).(RawProvider); !ok {
		t.Error("FreesoundProvider must implement RawProvider")
	}
	if _, ok := any(p).(MediaProvider); !ok {
		t.Error("Provider must implement MediaProvider")
	}
	if got := strings.Join(p.MediaTypes(), ","); got != "sfx" {
		t.Errorf("MediaTypes() = %q, want sfx", got)
	}
	if _, ok := any(p).(Previewer); !ok {
		t.Error("FreesoundProvider must implement Previewer")
	}
}

func TestFreesoundSearch(t *testing.T) {
	var gotPath, gotAuth string
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		io.WriteString(w, `{"count":2,"results":[
			{"id":123,"name":"door slam.wav","url":"https://freesound.org/s/123/",
			 "previews":{"preview-hq-mp3":"https://cdn.freesound.org/previews/123_hq.mp3",
			             "preview-lq-mp3":"https://cdn.freesound.org/previews/123_lq.mp3"}},
			{"id":456,"name":"rain loop.wav","url":"https://freesound.org/s/456/","previews":{}}
		]}`)
	}))
	defer srv.Close()

	p := &FreesoundProvider{apiKey: "fs-key", baseURL: srv.URL}
	results, raw, err := p.SearchRaw("door slam", 5, Options{})
	if err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}

	if gotPath != "/apiv2/search/" {
		t.Errorf("path = %q, want /apiv2/search/", gotPath)
	}
	if gotAuth != "Token fs-key" {
		t.Errorf("Authorization = %q, want Token fs-key", gotAuth)
	}
	if got := gotQuery.Get("query"); got != "door slam" {
		t.Errorf("query = %q, want door slam", got)
	}
	if got := gotQuery.Get("page_size"); got != "5" {
		t.Errorf("page_size = %q, want 5", got)
	}
	if got := gotQuery.Get("fields"); !strings.Contains(got, "previews") {
		t.Errorf("fields = %q, want previews included", got)
	}

	want := []Result{
		{URL: "https://cdn.freesound.org/previews/123_hq.mp3", Title: "door slam.wav"},
		{URL: "https://freesound.org/s/456/", Title: "rain loop.wav"},
	}
	if len(results) != len(want) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(want))
	}
	for i := range want {
		if results[i] != want[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], want[i])
		}
	}
	if !strings.Contains(string(raw), `"previews"`) {
		t.Errorf("raw response was not returned verbatim: %s", raw)
	}
}

func TestFreesoundInjectKeepsReservedParams(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		io.WriteString(w, `{"results":[]}`)
	}))
	defer srv.Close()

	p := &FreesoundProvider{apiKey: "fs-key", baseURL: srv.URL}
	opts := Options{Extra: json.RawMessage(`{"filter":"license:\"Creative Commons 0\" duration:[0 TO 3]","query":"hacked","page_size":99,"fields":"id"}`)}
	if _, _, err := p.SearchRaw("door slam", 5, opts); err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}

	if got := gotQuery.Get("filter"); got != `license:"Creative Commons 0" duration:[0 TO 3]` {
		t.Errorf("filter = %q, want the injected license/duration filter", got)
	}
	if got := gotQuery.Get("query"); got != "door slam" {
		t.Errorf("query = %q, want the CLI-owned query", got)
	}
	if got := gotQuery.Get("page_size"); got != "5" {
		t.Errorf("page_size = %q, want the CLI-owned limit", got)
	}
	if got := gotQuery.Get("fields"); !strings.Contains(got, "previews") {
		t.Errorf("fields = %q, want the CLI-owned field list", got)
	}
}

func TestFreesoundErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := &FreesoundProvider{apiKey: "fs-key", baseURL: srv.URL}
	if _, _, err := p.SearchRaw("q", 5, Options{}); err == nil {
		t.Fatal("SearchRaw() error = nil, want API error")
	}
}

func TestFreesoundPreviewMasksKey(t *testing.T) {
	p := NewFreesoundProvider("fs-key")
	plan, err := p.Preview("door slam", 5, Options{
		Extra: json.RawMessage(`{"filter":"license:\"Creative Commons 0\""}`),
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
	if got := u.Query().Get("filter"); got != `license:"Creative Commons 0"` {
		t.Errorf("filter = %q, want the injected filter", got)
	}
	for _, header := range plan.Headers {
		if header.Name != "Authorization" {
			continue
		}
		if header.Value == "Token fs-key" || header.Value == "Token " {
			t.Errorf("Authorization header = %q, want a masked key", header.Value)
		}
	}
}
