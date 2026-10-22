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

func TestNewPexelsProviderDefaults(t *testing.T) {
	p := NewPexelsProvider("px-key")
	if p.Name() != "pexels" {
		t.Errorf("Name() = %q, want pexels", p.Name())
	}
	if p.baseURL != defaultPexelsBaseURL {
		t.Errorf("baseURL = %q, want %q", p.baseURL, defaultPexelsBaseURL)
	}
	if p.baseURL != "https://api.pexels.com" {
		t.Errorf("default base URL = %q, want https://api.pexels.com", p.baseURL)
	}
}

func TestPexelsProviderImplementsCapabilities(t *testing.T) {
	p := NewPexelsProvider("px-key")
	if _, ok := any(p).(Provider); !ok {
		t.Error("PexelsProvider must implement Provider")
	}
	if _, ok := any(p).(RawProvider); !ok {
		t.Error("PexelsProvider must implement RawProvider")
	}
	if _, ok := any(p).(Previewer); !ok {
		t.Error("PexelsProvider must implement Previewer")
	}
	if _, ok := any(p).(MediaProvider); !ok {
		t.Error("PexelsProvider must implement MediaProvider")
	}
	if got := strings.Join(p.MediaTypes(), ","); got != "image,video" {
		t.Errorf("MediaTypes() = %q", got)
	}
}

func TestPexelsSearchImage(t *testing.T) {
	var gotPath, gotAuth string
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"photos":[
			{"url":"https://www.pexels.com/photo/a-1/","alt":"rain on glass","photographer":"Alice"},
			{"url":"https://www.pexels.com/photo/b-2/","alt":"","photographer":"Bob"}
		]}`)
	}))
	defer srv.Close()

	p := &PexelsProvider{apiKey: "px-key", baseURL: srv.URL}
	results, raw, err := p.SearchRaw("rain window", 5, Options{Type: "image"})
	if err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}

	if gotPath != "/v1/search" {
		t.Errorf("path = %q, want /v1/search", gotPath)
	}
	if gotAuth != "px-key" {
		t.Errorf("Authorization = %q, want px-key", gotAuth)
	}
	if got := gotQuery.Get("query"); got != "rain window" {
		t.Errorf("query = %q, want rain window", got)
	}
	if got := gotQuery.Get("per_page"); got != "5" {
		t.Errorf("per_page = %q, want 5", got)
	}

	want := []Result{
		{URL: "https://www.pexels.com/photo/a-1/", Title: "rain on glass"},
		{URL: "https://www.pexels.com/photo/b-2/", Title: "Bob"},
	}
	if len(results) != len(want) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(want))
	}
	for i := range want {
		if results[i] != want[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], want[i])
		}
	}
	if !strings.Contains(string(raw), `"photos"`) {
		t.Errorf("raw response was not returned verbatim: %s", raw)
	}
}

func TestPexelsSearchVideo(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		io.WriteString(w, `{"videos":[
			{"url":"https://www.pexels.com/video/a-1/","user":{"name":"Alice"}}
		]}`)
	}))
	defer srv.Close()

	p := &PexelsProvider{apiKey: "px-key", baseURL: srv.URL}
	results, _, err := p.SearchRaw("rain window", 5, Options{Type: "video"})
	if err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}
	if gotPath != "/videos/search" {
		t.Errorf("path = %q, want /videos/search", gotPath)
	}
	if got := gotQuery.Get("query"); got != "rain window" {
		t.Errorf("query = %q, want rain window", got)
	}
	if len(results) != 1 || results[0].URL != "https://www.pexels.com/video/a-1/" {
		t.Errorf("results = %+v, want the video page URL", results)
	}
}

func TestPexelsInjectAndReservedQuery(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		io.WriteString(w, `{"photos":[]}`)
	}))
	defer srv.Close()

	p := &PexelsProvider{apiKey: "px-key", baseURL: srv.URL}
	opts := Options{
		Type:  "image",
		Extra: json.RawMessage(`{"orientation":"landscape","size":"large","page":2,"query":"hacked","per_page":99}`),
	}
	if _, _, err := p.SearchRaw("rain window", 5, opts); err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}
	for key, want := range map[string]string{
		"orientation": "landscape",
		"size":        "large",
		"page":        "2",
		"query":       "rain window",
		"per_page":    "5",
	} {
		if got := gotQuery.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestPexelsUnsupportedType(t *testing.T) {
	p := NewPexelsProvider("px-key")
	if _, _, err := p.SearchRaw("q", 5, Options{Type: "illustration"}); err == nil {
		t.Fatal("SearchRaw() error = nil, want unsupported type error")
	}
}

func TestPexelsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := &PexelsProvider{apiKey: "px-key", baseURL: srv.URL}
	if _, _, err := p.SearchRaw("q", 5, Options{}); err == nil {
		t.Fatal("SearchRaw() error = nil, want API error")
	}
}

func TestPexelsPreviewMasksKey(t *testing.T) {
	p := NewPexelsProvider("px-key")
	plan, err := p.Preview("rain window", 5, Options{
		Type:  "video",
		Extra: json.RawMessage(`{"orientation":"landscape"}`),
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if plan.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", plan.Method)
	}
	if !strings.HasSuffix(plan.URL, "/videos/search?orientation=landscape&per_page=5&query=rain+window") {
		t.Errorf("URL = %q, want the video endpoint with injected filters", plan.URL)
	}
	var auth string
	for _, h := range plan.Headers {
		if h.Name == "Authorization" {
			auth = h.Value
		}
	}
	if auth == "px-key" || auth == "" {
		t.Errorf("preview Authorization = %q, want a masked non-empty value", auth)
	}
}

func TestPexelsPhotoAliasRejected(t *testing.T) {
	p := NewPexelsProvider("px-key")
	_, _, err := p.SearchRaw("q", 5, Options{Type: "photo"})
	if err == nil || !strings.Contains(err.Error(), "--type image") {
		t.Fatalf("error = %v, want guidance to use --type image", err)
	}
}
